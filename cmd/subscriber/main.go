package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path"
	"strconv"
	"strings"
	"syscall"
	"time"

	quickfix "github.com/cryptogarageinc/quickfix-go"
	"github.com/cryptogarageinc/quickfix-go/field"
	fix44quote "github.com/cryptogarageinc/quickfix-go/fix44/quote"
	fix44qr "github.com/cryptogarageinc/quickfix-go/fix44/quoterequest"
	fix44rs "github.com/cryptogarageinc/quickfix-go/fix44/resendrequest"
	"github.com/cryptogarageinc/quickfix-go/tag"
	// "github.com/shopspring/decimal"
)

type SubscribeMessage struct {
	setting   *quickfix.Settings
	sessionID quickfix.SessionID
	initiator *quickfix.Initiator
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

// newResendRequest returns ResendRequest message.
func (m *SubscribeMessage) newResendRequest(begin, end int) *quickfix.Message {
	order := fix44rs.New(field.NewBeginSeqNo(end), field.NewEndSeqNo(begin))
	order.Header.SetTargetCompID(m.sessionID.TargetCompID)
	order.Header.SetSenderCompID(m.sessionID.SenderCompID)
	return order.ToMessage()
}

type PriceLogger struct {
	Enable   bool
	FileName string
	Asset    string
	Exchange string
	Handle   *os.File
	Count    uint64
}

// NewPriceLogger This function create PriceLogger.
func NewPriceLogger(settings *quickfix.Settings) *PriceLogger {
	globalSetting := settings.GlobalSettings()
	enable, err := globalSetting.BoolSetting("LoggingPrice")
	if err != nil {
		enable = false
	}
	asset, err := globalSetting.Setting("LoggingAsset")
	if err != nil {
		asset = "BTC/JPY"
	}
	exchange, err := globalSetting.Setting("LoggingExchangeName")
	if err != nil {
		exchange = ""
	}
	filename, err := globalSetting.Setting("LoggingFileName")
	if err != nil || len(filename) == 0 {
		filename = "price_{asset}_{time}.csv"
	} else if !strings.Contains(filename, ".csv") {
		filename = filename + ".csv"
	}
	assetName := strings.Replace(asset, "/", "_", -1)
	timeString := time.Now().UTC().Format("20060102150405")
	filename = strings.Replace(filename, "{asset}", assetName, -1)
	filename = strings.Replace(filename, "{time}", timeString, -1)
	return &PriceLogger{
		Enable:   enable,
		FileName: filename,
		Asset:    asset,
		Exchange: exchange,
	}
}

// Open This function open logging file.
func (obj *PriceLogger) Open() error {
	if !obj.Enable {
		return nil
	}
	file, err := os.OpenFile(obj.FileName, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666)
	if err != nil {
		return err
	}

	_, err = file.WriteString(",time,exchange,qty,sellprice,buyprice\n")
	if err != nil {
		file.Close()
		return err
	}
	obj.Handle = file
	return nil
}

// Open This function close logging file.
func (obj *PriceLogger) Close() error {
	if obj.Handle == nil {
		return nil
	}
	return obj.Handle.Close()
}

// WriteQuote This function write quote message.
func (obj *PriceLogger) WriteQuoteMessage(quote *fix44quote.Quote) error {
	if obj.Handle == nil {
		return nil
	}
	asset, err := quote.GetSymbol()
	if err != nil {
		return err
	} else if obj.Asset != asset {
		return nil
	}
	count := obj.Count
	if count == uint64(0xffffffffffffffff) {
		obj.Count = 0
	} else {
		obj.Count = obj.Count + 1
	}
	timeData, err := quote.GetTransactTime()
	if err != nil {
		// if field is not found, use receive time.
		timeData = time.Now().UTC()
	}
	timeString := timeData.Format("2006-01-02 15:04:05.000000")
	qty, err := quote.GetBidSize()
	if err != nil {
		return err
	}
	bid, err := quote.GetBidPx() // sell
	if err != nil {
		return err
	}
	offer, err := quote.GetOfferPx() // ask
	if err != nil {
		return err
	}
	floatQty, _ := qty.Float64()
	floatBid, _ := bid.Float64()
	floatOffer, _ := offer.Float64()

	logStr := fmt.Sprintf("%d,%s,%s,%g,%f,%f\n", count, timeString, obj.Exchange, floatQty, floatBid, floatOffer)
	_, osError := obj.Handle.WriteString(logStr)
	return osError
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
	logger  *PriceLogger
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
		fmt.Printf("Recv Logon: %s\n", strings.Replace(msg.String(), "\u0001", "|", -1))
	} else if msgType == "5" {
		fmt.Printf("Recv Logout: %s\n", msg.String())
	} else if msgType == "3" {
		fmt.Printf("Recv Reject: %s\n", msg.String())
	} else if msgType == "4" {
		fmt.Printf("Recv SequenceReset: %s\n", msg)
	} else if msgType != "0" {
		fmt.Printf("Recv: %s\n", msg.String())
	} else if e.isDebug {
		fmt.Println("[heartbeat] 60 sleep start.")
		time.Sleep(120 * time.Second)
		fmt.Println("Recv heartbeat.")
	}
	return
}

//ToAdmin implemented as part of Application interface
func (e Subscriber) ToAdmin(msg *quickfix.Message, sessionID quickfix.SessionID) {
	msgType, err := msg.MsgType()
	if err != nil {
		fmt.Printf("Receive Invalid adminMsg: %s\n", msg.String())
	} else if !e.isDebug {
		// do nothing
	} else if msgType == "A" {
		fmt.Printf("Send Logon: %s\n", msg)
		time.Sleep(time.Second * 10)
	} else if msgType == "5" {
		fmt.Printf("Send Logout: %s\n", msg)
	} else if msgType == "2" {
		fmt.Printf("Send ResendRequest: %s\n", msg)
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
		// fmt.Println("[Quote] 120 sleep start.")
		// time.Sleep(120 * time.Second)
		quoteData := fix44quote.FromMessage(msg)
		fmt.Printf("Quote: %s, size=%d\n", msg.String(), quoteData.Body.Len())
		tmpErr := e.logger.WriteQuoteMessage(&quoteData)
		if tmpErr != nil {
			fmt.Printf("Logging error: %s\n", tmpErr)
		}
	} else if msgType == "j" {
		fmt.Printf("BusinessMessageReject: %s\n", msg.String())
	} else if msgType == "4" {
		fmt.Printf("Recv sequenceReset: %s\n", msg)
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
	return e.data.initiator.SendToAliveSession(order, e.data.sessionID)
}

// queryQuoteRequestOrder This function send QuoteRequest message.
func (e Subscriber) resendRequest(begin, end int) (err error) {
	defer func() {
		if e := recover(); e != nil {
			err = e.(error)
		}
	}()

	order := e.data.newResendRequest(begin, end)
	return e.data.initiator.SendToAliveSession(order, e.data.sessionID)
}

func main() { // タスクを定義
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
	beginResendIndexOnBoot := -1
	endResendIndexOnBoot := -1
	if appSettings.GlobalSettings().HasSetting("BeginResendIndexOnBoot") && appSettings.GlobalSettings().HasSetting("EndResendIndexOnBoot") {
		beginResendIndexOnBoot, _ = appSettings.GlobalSettings().IntSetting("BeginResendIndexOnBoot")
		endResendIndexOnBoot, _ = appSettings.GlobalSettings().IntSetting("EndResendIndexOnBoot")
	}

	appData := SubscribeMessage{setting: appSettings}
	logger := NewPriceLogger(appSettings)
	app := Subscriber{data: &appData, isDebug: isDebug, logger: logger}
	fileLogFactory, err := quickfix.NewFileLogFactory(appSettings)
	if err != nil {
		fmt.Println("Error creating file log factory,", err)
		return
	}
	storeFactory := quickfix.NewMemoryStoreFactory()
	if appSettings.GlobalSettings().HasSetting("FileStorePath") {
		storeFactory = quickfix.NewFileStoreFactory(appSettings)
	}
	initiator, err := quickfix.NewInitiator(app, storeFactory, appSettings, fileLogFactory)
	if err != nil {
		fmt.Printf("Unable to create Initiator: %s\n", err)
		return
	}

	err = logger.Open()
	if err != nil {
		fmt.Printf("Error opening log file: %s\n", err)
		return
	}
	defer logger.Close()

	app.data.initiator = initiator
	err = initiator.Start()
	if err != nil {
		fmt.Printf("Failed to Start: %s\n", err)
		return
	}
	defer func() {
		fmt.Printf("Call stop.\n")
		initiator.Stop()
		fmt.Printf("Called stop.\n")
	}()

	sessId := initiator.GetSessionIDs()[0]
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM) // os.Kill
	startTimer := time.NewTimer(time.Second * 5)
	defer startTimer.Stop()

	if isDebug {
		fmt.Printf("Wait1 start\n")
		select {
		case <-quickfix.WaitForLogon(sessId):
			fmt.Printf("Wait1 finish\n")
		case <-startTimer.C:
			fmt.Printf("Wait1 timeout\n")
		case <-interrupt:
			return
		}
	}
	fmt.Printf("Wait start\n")
	select {
	case <-quickfix.WaitForLogon(sessId):
		fmt.Printf("Wait finish\n")
		time.Sleep(1 * time.Second)
	case <-interrupt:
		return
	}
	if beginResendIndexOnBoot >= 0 && endResendIndexOnBoot >= 0 {
		err = app.resendRequest(beginResendIndexOnBoot, endResendIndexOnBoot)
		if err != nil {
			fmt.Printf("Failed to resendRequest: %s\n", err)
			return
		}
	}
	err = app.queryQuoteRequestOrder("CG001", "BTC/JPY", "BTC-1-00000000")
	if err != nil {
		fmt.Printf("Failed to queryQuoteRequestOrder: %s\n", err)
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
		select {
		case <-interrupt:
			fmt.Printf("recv interrupt\n")
			return
		default:
		}
	}
	fmt.Printf("end\n")
}
