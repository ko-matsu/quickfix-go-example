package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path"
	"strconv"

	quickfix "github.com/cryptogarageinc/quickfix-go"
	"github.com/cryptogarageinc/quickfix-go/field"
	fix44quote "github.com/cryptogarageinc/quickfix-go/fix44/quote"
	fix44qr "github.com/cryptogarageinc/quickfix-go/fix44/quoterequest"
	"github.com/cryptogarageinc/quickfix-go/tag"
	// "github.com/shopspring/decimal"
)

type SubscribeMessage struct {
	setting   *quickfix.Settings
	sessionID quickfix.SessionID
}

func (m *SubscribeMessage) SetSession(sessionID quickfix.SessionID) {
	m.sessionID = sessionID
}

// newQuoteRequestByFix44 This function create QuoteRequest message.
func (m *SubscribeMessage) newQuoteRequestByFix44(quoteReqID, symbol, account string) *quickfix.Message {
	order := fix44qr.New(field.NewQuoteReqID(quoteReqID))
	// order.Set(field.NewAccount(account))
	// order.Set(field.NewSymbol(symbol))
	// order.Set(field.NewQuoteRequestType(enum.QuoteRequestType_AUTOMATIC))
	// order.Set(field.NewQuoteType(enum.QuoteType_RESTRICTED_TRADEABLE))
	// order.Set(field.NewOrdType(enum.OrdType_FOREX_MARKET))
	// FIXME
	// order.Set(field.NewOrderQty(decimal.New(0, 0), 0))
	group := fix44qr.NewNoRelatedSymRepeatingGroup()
	groupData := group.Add().Group
	groupData.FieldMap.SetString(tag.Account, account)
	groupData.FieldMap.SetString(tag.Symbol, symbol)
	groupData.FieldMap.SetInt(tag.OrderQty, 0)
	order.SetNoRelatedSym(group)
	// order.Set(field.NewNoRelatedSym(1))

	order.Header.SetTargetCompID(m.sessionID.TargetCompID)
	order.Header.SetSenderCompID(m.sessionID.SenderCompID)
	return order.ToMessage()
}

type QuoteRequestData struct {
	QuoteReqID, Symbol, Account string
	Index                       int
}

func GetQuoteRequestDatas(settings *quickfix.Settings) []QuoteRequestData {
	list := make([]QuoteRequestData, 0, 8)
	datas := settings.GlobalSettings()
	for i := 1; i < 9; i++ {
		numStr := strconv.Itoa(i)
		quoteReqId, err := datas.Setting("QuoteReqId" + numStr)
		if err != nil {
			continue
		}
		symbol, err := datas.Setting("Symbol" + numStr)
		if err != nil {
			continue
		}
		account, err := datas.Setting("Account" + numStr)
		if err != nil {
			continue
		}
		list = append(list, QuoteRequestData{
			QuoteReqID: quoteReqId,
			Symbol:     symbol,
			Account:    account,
			Index:      i,
		})
	}
	return list
}

//Subscriber implements the quickfix.Application interface
type Subscriber struct {
	data    *SubscribeMessage
	isDebug bool
}

//OnCreate implemented as part of Application interface
func (e Subscriber) OnCreate(sessionID quickfix.SessionID) {
	e.data.sessionID = sessionID
}

//OnLogon implemented as part of Application interface
func (e Subscriber) OnLogon(sessionID quickfix.SessionID) {
	fmt.Println("Logon.")
}

//OnLogout implemented as part of Application interface
func (e Subscriber) OnLogout(sessionID quickfix.SessionID) {
	fmt.Println("Logout.")
}

//FromAdmin implemented as part of Application interface
func (e Subscriber) FromAdmin(msg *quickfix.Message, sessionID quickfix.SessionID) (reject quickfix.MessageRejectError) {
	msgType, err := msg.MsgType()
	if err != nil {
		fmt.Printf("Receive Invalid adminMsg: %s\n", msg.String())
	} else if msgType == "A" {
		fmt.Printf("Recv Logon: %s\n", msg.String())
	} else if msgType == "5" {
		fmt.Printf("Recv Logout: %s\n", msg.String())
	} else if msgType == "3" {
		fmt.Printf("Recv Reject: %s\n", msg.String())
	} else if msgType != "0" {
		fmt.Printf("Recv: %s\n", msg.String())
	} else if e.isDebug {
		fmt.Println("Recv heartbeat.")
	}
	return
}

//ToAdmin implemented as part of Application interface
func (e Subscriber) ToAdmin(msg *quickfix.Message, sessionID quickfix.SessionID) {
	msgType, err := msg.MsgType()
	if err != nil {
		fmt.Printf("Receive Invalid adminMsg: %s\n", msg.String())
	} else if e.isDebug == false {
		// do nothing
	} else if msgType != "0" {
		fmt.Printf("Send: %s\n", msg)
	}
}

//ToApp implemented as part of Application interface
func (e Subscriber) ToApp(msg *quickfix.Message, sessionID quickfix.SessionID) (err error) {
	fmt.Printf("Sending %s\n", msg)
	return
}

//FromApp implemented as part of Application interface. This is the callback for all Application level messages from the counter party.
func (e Subscriber) FromApp(msg *quickfix.Message, sessionID quickfix.SessionID) (reject quickfix.MessageRejectError) {
	msgType, err := msg.MsgType()
	if err != nil {
		fmt.Printf("Receive Invalid msg: %s\n", msg.String())
	} else if msgType == "S" {
		quoteData := fix44quote.FromMessage(msg)
		fmt.Printf("Quote: %s, size=%d\n", msg.String(), quoteData.Body.Len())
	} else if msgType == "j" {
		fmt.Printf("BusinessMessageReject: %s\n", msg.String())
	} else {
		fmt.Printf("Receive: %s\n", msg.String())
	}
	return
}

// queryQuoteRequestOrder This function send QuoteRequest message.
func (e Subscriber) queryQuoteRequestOrder(quoteReqID, symbol, account string) (err error) {
	defer func() {
		if e := recover(); e != nil {
			err = e.(error)
		}
	}()

	order := e.data.newQuoteRequestByFix44(quoteReqID, symbol, account)
	return quickfix.Send(order)
}

func main() {
	flag.Parse()

	cfgFileName := path.Join("config", "subscriber.cfg")
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
	isDebug := false
	if appSettings.GlobalSettings().HasSetting("Debug") {
		isDebug, _ = appSettings.GlobalSettings().BoolSetting("Debug")
	}

	appData := SubscribeMessage{setting: appSettings}
	app := Subscriber{data: &appData, isDebug: isDebug}
	fileLogFactory, err := quickfix.NewFileLogFactory(appSettings)

	if err != nil {
		fmt.Println("Error creating file log factory,", err)
		return
	}

	initiator, err := quickfix.NewInitiator(app, quickfix.NewMemoryStoreFactory(), appSettings, fileLogFactory)
	if err != nil {
		fmt.Printf("Unable to create Initiator: %s\n", err)
		return
	}

	err = initiator.Start()
	if err != nil {
		fmt.Printf("Failed to Start: %s\n", err)
		return
	}

	quoteList := GetQuoteRequestDatas(appSettings)

Loop:
	for {
		fmt.Println()
		for _, quoteData := range quoteList {
			fmt.Printf("%d) Quote Request(%s)\n", quoteData.Index, quoteData.Symbol)
		}
		fmt.Println("9) Quit")
		fmt.Print("Action > ")
		scanner := bufio.NewScanner(os.Stdin)
		scanner.Scan()
		action := scanner.Text()
		if scanner.Err() != nil {
			fmt.Println("Scan Error:", scanner.Err())
			break
		}
		if action == "9" {
			break Loop // exit
		}

		isFind := false
		index, err := strconv.Atoi(action)
		for _, quoteData := range quoteList {
			if quoteData.Index == index {
				err = app.queryQuoteRequestOrder(quoteData.QuoteReqID, quoteData.Symbol, quoteData.Account)
				isFind = true
				break
			}
		}
		if !isFind {
			err = fmt.Errorf("unknown action: '%v'", action)
		}
		if err != nil {
			fmt.Printf("%v\n", err)
		}
	}

	initiator.Stop()
}
