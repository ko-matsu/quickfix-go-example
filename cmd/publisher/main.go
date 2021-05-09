package main

import (
	"flag"
	"fmt"
	"path"
	"syscall"
	"time"

	quickfix "github.com/cryptogarageinc/quickfix-go"
	"github.com/cryptogarageinc/quickfix-go/enum"
	"github.com/cryptogarageinc/quickfix-go/field"
	"github.com/cryptogarageinc/quickfix-go/tag"
	"github.com/shopspring/decimal"

	fix44er "github.com/cryptogarageinc/quickfix-go/fix44/executionreport"
	fix44nos "github.com/cryptogarageinc/quickfix-go/fix44/newordersingle"
	fix44quote "github.com/cryptogarageinc/quickfix-go/fix44/quote"
	fix44qr "github.com/cryptogarageinc/quickfix-go/fix44/quoterequest"

	"os"
	"os/signal"
	"strconv"
)

type publisher struct {
	orderID int
	execID  int
	*quickfix.MessageRouter
}

func newPublisher() *publisher {
	p := &publisher{MessageRouter: quickfix.NewMessageRouter()}
	p.AddRoute(fix44nos.Route(p.OnFIX44NewOrderSingle))
	p.AddRoute(fix44qr.Route(p.OnFIX44NewQuoteRequest))
	return p
}

func (e *publisher) genOrderID() field.OrderIDField {
	e.orderID++
	return field.NewOrderID(strconv.Itoa(e.orderID))
}

func (e *publisher) genExecID() field.ExecIDField {
	e.execID++
	return field.NewExecID(strconv.Itoa(e.execID))
}

//quickfix.Application interface
func (e publisher) OnCreate(sessionID quickfix.SessionID)                           {}
func (e publisher) OnLogon(sessionID quickfix.SessionID)                            {}
func (e publisher) OnLogout(sessionID quickfix.SessionID)                           {}
func (e publisher) ToAdmin(msg *quickfix.Message, sessionID quickfix.SessionID)     {}
func (e publisher) ToApp(msg *quickfix.Message, sessionID quickfix.SessionID) error { return nil }
func (e publisher) FromAdmin(msg *quickfix.Message, sessionID quickfix.SessionID) quickfix.MessageRejectError {
	return nil
}

//Use Message Cracker on Incoming Application Messages
func (e *publisher) FromApp(msg *quickfix.Message, sessionID quickfix.SessionID) (reject quickfix.MessageRejectError) {
	return e.Route(msg, sessionID)
}

func (e *publisher) OnFIX44NewOrderSingle(msg fix44nos.NewOrderSingle, sessionID quickfix.SessionID) (err quickfix.MessageRejectError) {
	ordType, err := msg.GetOrdType()
	if err != nil {
		return err
	}

	if ordType != enum.OrdType_LIMIT {
		return quickfix.ValueIsIncorrect(tag.OrdType)
	}

	symbol, err := msg.GetSymbol()
	if err != nil {
		return
	}

	side, err := msg.GetSide()
	if err != nil {
		return
	}

	orderQty, err := msg.GetOrderQty()
	if err != nil {
		return
	}

	price, err := msg.GetPrice()
	if err != nil {
		return
	}

	clOrdID, err := msg.GetClOrdID()
	if err != nil {
		return
	}

	execReport := fix44er.New(
		e.genOrderID(),
		e.genExecID(),
		field.NewExecType(enum.ExecType_FILL),
		field.NewOrdStatus(enum.OrdStatus_FILLED),
		field.NewSide(side),
		field.NewLeavesQty(decimal.Zero, 2),
		field.NewCumQty(orderQty, 2),
		field.NewAvgPx(price, 2),
	)

	execReport.SetClOrdID(clOrdID)
	execReport.SetSymbol(symbol)
	execReport.SetOrderQty(orderQty, 2)
	execReport.SetLastQty(orderQty, 2)
	execReport.SetLastPx(price, 2)

	if msg.HasAccount() {
		acct, err := msg.GetAccount()
		if err != nil {
			return err
		}
		execReport.SetAccount(acct)
	}

	quickfix.SendToTarget(execReport, sessionID)

	return
}

func (e *publisher) OnFIX44NewQuoteRequest(msg fix44qr.QuoteRequest, sessionID quickfix.SessionID) (err quickfix.MessageRejectError) {
	quoteReqID, err := msg.GetQuoteReqID()
	if err != nil {
		return
	}
	symbol, err := msg.GetString(tag.Symbol)
	if err != nil {
		return
	}

	quote := fix44quote.New(field.NewQuoteID("TEST"))
	quote.SetQuoteReqID(quoteReqID)
	quote.SetCurrency("BTC")
	quote.SetTransactTime(time.Now())
	quote.SetSymbol(symbol)
	quote.SetBidPx(decimal.New(120, 0), 2)
	quote.SetOfferPx(decimal.New(100, 0), 2)
	quote.SetBidSize(decimal.New(120, 0), 2)
	quote.SetOfferSize(decimal.New(100, 0), 2)

	if msg.Has(tag.Account) {
		account, err := msg.GetString(tag.Account)
		if err != nil {
			return err
		}
		quote.SetAccount(account)
	}

	quickfix.SendToTarget(quote, sessionID)
	return
}

func main() {
	flag.Parse()

	cfgFileName := path.Join("config", "executor.cfg")
	if flag.NArg() > 0 {
		cfgFileName = flag.Arg(0)
	}

	cfg, err := os.Open(cfgFileName)
	if err != nil {
		fmt.Printf("Error opening %v, %v\n", cfgFileName, err)
		return
	}

	appSettings, err := quickfix.ParseSettings(cfg)
	if err != nil {
		fmt.Println("Error reading cfg,", err)
		return
	}

	logFactory := quickfix.NewScreenLogFactory()
	app := newPublisher()

	acceptor, err := quickfix.NewAcceptor(app, quickfix.NewMemoryStoreFactory(), appSettings, logFactory)
	if err != nil {
		fmt.Printf("Unable to create Acceptor: %s\n", err)
		return
	}

	err = acceptor.Start()
	if err != nil {
		fmt.Printf("Unable to start Acceptor: %s\n", err)
		return
	}

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM) // os.Kill
	<-interrupt

	acceptor.Stop()
}
