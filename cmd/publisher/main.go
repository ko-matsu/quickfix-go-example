package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path"
	"strconv"
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

	_ "github.com/mattn/go-sqlite3"
)

type acceptorObject struct {
	*quickfix.Acceptor
	*quickfix.Settings
	BeginString  string
	SenderCompID string
}

type publisher struct {
	orderID int
	execID  int
	*quickfix.MessageRouter
	*acceptorObject
	errorSessionMap map[quickfix.SessionID]int
}

func newPublisher() *publisher {
	p := &publisher{
		MessageRouter:   quickfix.NewMessageRouter(),
		acceptorObject:  &acceptorObject{},
		errorSessionMap: make(map[quickfix.SessionID]int),
	}
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
func (e publisher) OnCreate(sessionID quickfix.SessionID) {
	fmt.Printf("OnCreate: %s\n", sessionID.String())
}

func (e publisher) OnLogon(sessionID quickfix.SessionID) {
	fmt.Printf("OnLogon: %s\n", sessionID.String())
}

func (e publisher) OnLogout(sessionID quickfix.SessionID) {
	fmt.Printf("OnLogout: %s\n", sessionID.String())
}

func (e publisher) ToAdmin(msg *quickfix.Message, sessionID quickfix.SessionID)     {}
func (e publisher) ToApp(msg *quickfix.Message, sessionID quickfix.SessionID) error { return nil }

func (e publisher) FromAdmin(msg *quickfix.Message, sessionID quickfix.SessionID) quickfix.MessageRejectError {
	msgType, err := msg.MsgType()
	if err != nil {
		fmt.Printf("Receive Invalid adminMsg: %s\n", msg.String())
	} else if msgType == "A" {
		// if sessionID.TargetCompID !=
		fmt.Printf("Recv Logon: %s\n", msg.String())
	} else if msgType == "5" {
		fmt.Printf("Recv Logout: %s\n", msg.String())
	} else if msgType == "3" {
		fmt.Printf("Recv Reject: %s\n", msg.String())
	} else if msgType != "0" {
		fmt.Printf("Recv: %s\n", msg.String())
	} else {
		fmt.Println("Recv heartbeat.")
	}
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

	tempErr := e.acceptorObject.Acceptor.SendToAliveSession(execReport, sessionID)
	if tempErr != nil {
		fmt.Println("Error SendToAliveSession,", tempErr)
	}
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

	tempErr := e.acceptorObject.Acceptor.SendToAliveSession(quote, sessionID)
	if tempErr != nil {
		fmt.Println("Error SendToAliveSession,", tempErr)
	}
	return
}

func (e *publisher) OnFIX44NewQuoteAll() (err quickfix.MessageRejectError) {
	acceptor := e.acceptorObject.Acceptor
	list := acceptor.GetAliveSessionIDs()
	for _, sessionId := range list {

		quote := fix44quote.New(field.NewQuoteID("TEST"))
		quote.SetQuoteReqID("test")
		quote.SetCurrency("BTC")
		quote.SetTransactTime(time.Now())
		quote.SetSymbol("symbol")
		quote.SetBidPx(decimal.New(120, 0), 2)
		quote.SetOfferPx(decimal.New(100, 0), 2)
		quote.SetBidSize(decimal.New(120, 0), 2)
		quote.SetOfferSize(decimal.New(100, 0), 2)

		quote.SetBeginString(e.acceptorObject.BeginString)
		quote.SetSenderCompID(sessionId.SenderCompID)
		quote.SetTargetCompID(sessionId.TargetCompID)

		tempErr := acceptor.SendToAliveSession(quote, sessionId)
		if tempErr != nil {
			fmt.Println("Error SendToAliveSession,", tempErr)
		}
	}
	return
}

func (e *publisher) OnFIX44NewQuoteAll1() (err quickfix.MessageRejectError) {
	acceptor := e.acceptorObject.Acceptor

	quote := fix44quote.New(field.NewQuoteID("TEST"))
	quote.SetQuoteReqID("test")
	quote.SetCurrency("BTC")
	quote.SetTransactTime(time.Now())
	quote.SetSymbol("symbol")
	quote.SetBidPx(decimal.New(120, 0), 2)
	quote.SetOfferPx(decimal.New(100, 0), 2)
	quote.SetBidSize(decimal.New(120, 0), 2)
	quote.SetOfferSize(decimal.New(100, 0), 2)

	tempErr := acceptor.SendToAliveSessions(quote)
	if tempErr != nil {
		fmt.Println("Error SendToAliveSessions,", tempErr.Error())
		errMap := tempErr.(*quickfix.ErrorBySessionID)
		for key, value := range errMap.ErrorMap {
			fmt.Printf(" - session: %s, err: %v\n", key.String(), value)
		}
	}
	return
}

func (e *publisher) OnFIX44NewQuoteAll2() (err quickfix.MessageRejectError) {
	// acceptor := e.acceptorObject.Acceptor
	quote := fix44quote.New(field.NewQuoteID("TEST2"))
	quote.SetQuoteReqID("test2")
	quote.SetCurrency("BTC")
	quote.SetTransactTime(time.Now())
	quote.SetSymbol("symbol2")
	quote.SetBidPx(decimal.New(120, 0), 2)
	quote.SetOfferPx(decimal.New(100, 0), 2)
	quote.SetBidSize(decimal.New(120, 0), 2)
	quote.SetOfferSize(decimal.New(100, 0), 2)
	tempErr := quickfix.SendToAliveSessions(quote)
	if tempErr != nil {
		fmt.Println("Error SendToAliveSessions,", tempErr.Error())
		errMap := tempErr.(*quickfix.ErrorBySessionID)
		for key, value := range errMap.ErrorMap {
			fmt.Printf(" - session: %s, err: %v\n", key.String(), value)
		}
	}
	return
}

func (e *publisher) OnFIX44NewQuoteAll3() (err quickfix.MessageRejectError) {
	list := quickfix.GetAliveSessionIDs()
	for _, sessionId := range list {

		quote := fix44quote.New(field.NewQuoteID("TEST"))
		quote.SetQuoteReqID("test")
		quote.SetCurrency("BTC")
		quote.SetTransactTime(time.Now())
		quote.SetSymbol("symbol")
		quote.SetBidPx(decimal.New(120, 0), 2)
		quote.SetOfferPx(decimal.New(100, 0), 2)
		quote.SetBidSize(decimal.New(120, 0), 2)
		quote.SetOfferSize(decimal.New(100, 0), 2)

		quote.SetBeginString(e.acceptorObject.BeginString)
		quote.SetSenderCompID(sessionId.SenderCompID)
		quote.SetTargetCompID(sessionId.TargetCompID)

		tempErr := quickfix.SendToAliveSession(quote, sessionId)
		if tempErr != nil {
			fmt.Println("Error SendToAliveSession,", tempErr)
		}
	}
	return
}

func (e *publisher) OnFIX44NewQuoteAllByError() (err quickfix.MessageRejectError) {
	list := quickfix.GetAliveSessionIDs()
	for _, sessionId := range list {
		e.errorSessionMap[sessionId] = 0
	}
	for sessionId, errorCount := range e.errorSessionMap {
		if errorCount >= 3 {
			continue
		}
		quote := fix44quote.New(field.NewQuoteID("TEST"))
		quote.SetQuoteReqID("test")
		quote.SetCurrency("BTC")
		quote.SetTransactTime(time.Now())
		quote.SetSymbol("symbol")
		quote.SetBidPx(decimal.New(120, 0), 2)
		quote.SetOfferPx(decimal.New(100, 0), 2)
		quote.SetBidSize(decimal.New(120, 0), 2)
		quote.SetOfferSize(decimal.New(100, 0), 2)

		quote.SetBeginString(e.acceptorObject.BeginString)
		quote.SetSenderCompID(sessionId.SenderCompID)
		quote.SetTargetCompID(sessionId.TargetCompID)

		tempErr := quickfix.SendToSession(quote, sessionId)
		if tempErr != nil {
			fmt.Printf("Error(%s,%d) SendToSession,%v\n", sessionId.TargetCompID, errorCount+1, tempErr)
			e.errorSessionMap[sessionId] = errorCount + 1
		}
	}
	return
}

func (e *publisher) StoreQuoteMessageAll() (err quickfix.MessageRejectError) {
	sessionIds := []quickfix.SessionID{
		{
			BeginString:  "FIX.4.4",
			TargetCompID: "INIT1",
			SenderCompID: "ACCEPTOR",
		},
		{
			BeginString:  "FIX.4.4",
			TargetCompID: "INIT2",
			SenderCompID: "ACCEPTOR",
		},
	}
	for _, sessionId := range sessionIds {
		quote := fix44quote.New(field.NewQuoteID("StoreTEST"))
		quote.SetQuoteReqID("test")
		quote.SetCurrency("BTC")
		quote.SetTransactTime(time.Now())
		quote.SetSymbol("symbol")
		quote.SetBidPx(decimal.New(120, 0), 2)
		quote.SetOfferPx(decimal.New(100, 0), 2)
		quote.SetBidSize(decimal.New(120, 0), 2)
		quote.SetOfferSize(decimal.New(100, 0), 2)

		quote.SetBeginString(e.acceptorObject.BeginString)
		quote.SetSenderCompID(sessionId.SenderCompID)
		quote.SetTargetCompID(sessionId.TargetCompID)

		tempErr := quickfix.StoreMessageToSession(quote, sessionId)
		if tempErr != nil {
			fmt.Printf("Error(%s) StoreMessageToSession,%v\n", sessionId.TargetCompID, tempErr)
		}
	}
	return
}

func main() {
	flag.Parse()

	cfgFileName := path.Join("config", "executor.cfg")
	if flag.NArg() > 0 {
		cfgFileName = flag.Arg(0)
	}
	storeMessageFlag := false
	if flag.NArg() > 1 {
		msg := flag.Arg(1)
		if msg == "StoreMessage" {
			storeMessageFlag = true
		}
	}

	cfg, err := os.Open(cfgFileName)
	if err != nil {
		fmt.Printf("Error opening %v, %v\n", cfgFileName, err)
		return
	}

	appSettings, err := quickfix.ParseSettings(cfg)
	if err != nil {
		//if err.Error() == "no sessions declared" {
		fmt.Println("Error reading cfg,", err)
		return
	}

	logFactory := quickfix.NewScreenLogFactory()
	// TODO(k-matsuzawa): file logger is not supported dynamic session.
	// logFactory, err := quickfix.NewFileLogFactory(appSettings)
	// if err != nil {
	// 	fmt.Println("Error creating file log factory,", err)
	// 	return
	// }

	app := newPublisher()
	app.acceptorObject.Settings = appSettings
	app.acceptorObject.BeginString, err = appSettings.GlobalSettings().Setting("BeginString")
	if err != nil {
		fmt.Println("Error BeginString cfg,", err)
		return
	}

	hasGlobal, _ := appSettings.GlobalSettings().BoolSetting("DynamicSessions")
	app.acceptorObject.SenderCompID, err = appSettings.GlobalSettings().Setting("SenderCompID")
	if err != nil {
		if !hasGlobal {
			fmt.Println("Error SenderCompID cfg,", err)
			return
		}
	}

	storeFactory := quickfix.NewMemoryStoreFactory()
	if appSettings.GlobalSettings().HasSetting("FileStorePath") {
		storeFactory = quickfix.NewFileStoreFactory(appSettings)
	} else if appSettings.GlobalSettings().HasSetting("SQLStoreDriver") {
		storeFactory = quickfix.NewSQLTxStoreFactory(appSettings)
	}
	acceptor, err := quickfix.NewAcceptor(app, storeFactory, appSettings, logFactory)
	if err != nil {
		fmt.Printf("Unable to create Acceptor: %s\n", err)
		return
	}
	app.acceptorObject.Acceptor = acceptor

	if err = acceptor.Start(); err != nil {
		fmt.Printf("Unable to start Acceptor: %s\n", err)
		return
	}
	defer func() {
		acceptor.Stop()
		fmt.Println("Acceptor stop finish.")
	}()
	fmt.Println("Acceptor start.")

	go func() {
		ids := acceptor.GetSessionIDs()
		if len(ids) == 0 || hasGlobal {
			return
		}
		for _, id := range ids {
			go func(sessionId quickfix.SessionID) {
				interrupt := make(chan os.Signal, 1)
				signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM) // os.Kill
				for {
					fmt.Printf("## Wait start: %s\n", sessionId.String())
					select {
					case <-quickfix.WaitForLogon(sessionId):
						fmt.Printf("## Wait finish. Logon: %s\n", sessionId.String())
					case <-interrupt:
						fmt.Printf("## Wait interrupt: %s\n", sessionId.String())
						return
					}
				}
			}(id)
		}
	}()

	go func() {
		time.Sleep(5 * time.Second)
		if storeMessageFlag {
			app.StoreQuoteMessageAll()
		}
		for app.acceptorObject.Acceptor != nil {
			// app.OnFIX44NewQuoteAll()
			//app.OnFIX44NewQuoteAll1()
			// app.OnFIX44NewQuoteAll2()
			// app.OnFIX44NewQuoteAll3()
			app.OnFIX44NewQuoteAllByError()
			time.Sleep(20 * time.Second)
		}
	}()

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM) // os.Kill
	<-interrupt

	app.acceptorObject.Acceptor = nil
	fmt.Println("Acceptor stop.")
}
