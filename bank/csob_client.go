package bank

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"sort"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"

	database "github.com/vlcak/groupme_qr_bot/db"
)

const (
	TEMP_TRANSACTIONS_FILE = "transactions.json"
)

func NewCsobClient(accountNumber int, db *database.Client) *CsobClient {
	return &CsobClient{
		accountNumber: accountNumber,
		url:           fmt.Sprintf("https://csob.cz/firmy/bezne-ucty/transparentni-ucty/ucet?account=%d", accountNumber),
		db:            db,
		viewURL:       fmt.Sprintf("https://www.csob.cz/portal/firmy/bezne-ucty/transparentni-ucty/ucet?account=%d", accountNumber),
	}
}

type CsobClient struct {
	accountNumber int
	url           string
	db            *database.Client
	viewURL       string
}

type Payment struct {
	Name          string
	AccountNumber string
	Message       string
	Amount        int
	Order         int
	Timestamp     time.Time
}

func (cc *CsobClient) CheckPayments() ([]Payment, error) {
	previousLastAccountingOrder, err := cc.db.GetLastPaymentOrder()
	if err != nil {
		log.Printf("Can't get last accounting order: %v", err)
		return nil, err
	}
	log.Printf("Getting payments since: %d", previousLastAccountingOrder)
	payments, err := cc.paymentsSinceLastCheck(previousLastAccountingOrder)
	if err != nil {
		log.Printf("Can't get payments: %v", err)
		return nil, err
	}
	// Store payments to DB
	for _, payment := range payments {
		err = cc.db.StorePayment(payment.Name, payment.AccountNumber, payment.Amount, payment.Order, payment.Timestamp)
		if err != nil {
			log.Printf("Can't store payment: %v", err)
			return nil, err
		}
	}

	return payments, nil
}

func (cc *CsobClient) CheckPaymentsFromFile() ([]Payment, error) {
	previousLastAccountingOrder, err := cc.db.GetLastPaymentOrder()
	if err != nil {
		log.Printf("Can't get last accounting order: %v", err)
		return nil, err
	}
	log.Printf("Getting payments since: %d", previousLastAccountingOrder)
	payments, err := cc.paymentsSinceLastCheckFromFile(previousLastAccountingOrder, TEMP_TRANSACTIONS_FILE)
	if err != nil {
		log.Printf("Can't get payments: %v", err)
		return nil, err
	}
	// Store payments to DB
	for _, payment := range payments {
		err = cc.db.StorePayment(payment.Name, payment.AccountNumber, payment.Amount, payment.Order, payment.Timestamp)
		if err != nil {
			log.Printf("Can't store payment: %v", err)
			return nil, err
		}
	}

	return payments, nil
}

func (cc *CsobClient) GetAccountURL() string {
	return cc.viewURL
}

func (cc *CsobClient) PaymentsSinceLastCheck(lastAccountingOrder int) ([]Payment, error) {
	payments, err := cc.paymentsSinceLastCheck(lastAccountingOrder)
	if err != nil {
		log.Printf("Can't get payments: %v", err)
		return nil, err
	}
	return payments, nil
}

type bankResponse struct {
	AccountedTransaction []transaction
	Paging               struct {
		PageCount   int
		PageNumber  int
		RecordCount int
	}
}

type transaction struct {
	BaseInfo struct {
		AccountAmountData struct {
			Amount       int
			CurrencyCode string
		}
		AccountingOrder int
		AccountingDate  string
	}
	TransactionTypeChoice struct {
		DomesticPayment struct {
			Message struct {
				Message1 string
			}
			PartyAccount struct {
				DomesticAccount struct {
					AccountNumber int
					BankCode      string
				}
			}
			PartyName string
		}
	}
}

func (cc *CsobClient) paymentsSinceLastCheck(lastAccountingOrder int) ([]Payment, error) {
	dir, err := os.MkdirTemp("", "csob")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.DisableGPU,
		chromedp.NoDefaultBrowserCheck,
		chromedp.Flag("headless", false),
		chromedp.Flag("ignore-certificate-errors", true),
		chromedp.Flag("window-size", "50,400"),
		chromedp.UserDataDir(dir),
	)

	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()

	// also set up a custom logger
	taskCtx, cancel := chromedp.NewContext(allocCtx, chromedp.WithLogf(log.Printf))
	defer cancel()

	// create a timeout
	taskCtx, cancel = context.WithTimeout(taskCtx, 10*time.Second)
	defer cancel()

	// ensure that the browser process is started
	if err := chromedp.Run(taskCtx); err != nil {
		return nil, err
	}

	// listen network event
	pReference := listenForNetworkEvent(taskCtx, 1439)

	chromedp.Run(taskCtx,
		network.Enable(),
		chromedp.Navigate(cc.url),
		chromedp.Sleep(10*time.Second),
		chromedp.WaitVisible(`body`, chromedp.BySearch),
	)

	payments := *pReference

	sort.Slice(payments, func(i, j int) bool {
		return payments[i].Order < payments[j].Order
	})

	return payments, nil
}

func listenForNetworkEvent(ctx context.Context, lastAccountingOrder int) *[]Payment {
	var payments []Payment
	chromedp.ListenTarget(ctx, func(ev interface{}) {
		switch ev := ev.(type) {

		case *network.EventResponseReceived:
			resp := ev.Response
			if len(resp.Headers) != 0 {
				if resp.Headers["Content-Type"] != "application/json" {
					return
				}
			}
			go func() {
				c := chromedp.FromContext(ctx)
				rbp := network.GetResponseBody(ev.RequestID)
				body, err := rbp.Do(cdp.WithExecutor(ctx, c.Target))
				if err != nil {
					return
				}
				bankResponse := bankResponse{}

				err = json.Unmarshal(body, &bankResponse)
				if err != nil {
					log.Printf("Can't unmarshal bank response body: %v\nbody: %v", err, string(body))
					return
				}

				if bankResponse.Paging.PageNumber > 0 {
					payments = append(payments, processTransactions(bankResponse.AccountedTransaction, lastAccountingOrder)...)
				}
			}()
		}
	})
	return &payments
}

func processTransactions(transactions []transaction, lastAccountingOrder int) []Payment {
	var payments []Payment
	for _, transaction := range transactions {
		if transaction.BaseInfo.AccountingOrder > lastAccountingOrder && transaction.BaseInfo.AccountAmountData.Amount > 0 {
			var accountingDate time.Time
			p := Payment{
				Name: transaction.TransactionTypeChoice.DomesticPayment.PartyName,
				AccountNumber: fmt.Sprintf(
					"%d/%s",
					transaction.TransactionTypeChoice.DomesticPayment.PartyAccount.DomesticAccount.AccountNumber,
					transaction.TransactionTypeChoice.DomesticPayment.PartyAccount.DomesticAccount.BankCode,
				),
				Message: transaction.TransactionTypeChoice.DomesticPayment.Message.Message1,
				Amount:  transaction.BaseInfo.AccountAmountData.Amount,
				Order:   transaction.BaseInfo.AccountingOrder,
			}
			accountingDate, err := time.Parse("2006-01-02T15:04:05.000Z", transaction.BaseInfo.AccountingDate)
			if err != nil {
				log.Printf("Can't parse accounting date: %v", err)
				accountingDate = time.Now()
			}
			p.Timestamp = accountingDate
			payments = append(payments, p)
		}
	}
	return payments
}

func (cc *CsobClient) paymentsSinceLastCheckFromFile(lastAccountingOrder int, fileName string) ([]Payment, error) {
	if _, err := os.Stat(fileName); os.IsNotExist(err) {
		log.Printf("File not found: %s", fileName)
		return nil, err
	}

	file, err := os.Open(fileName)
	if err != nil {
		log.Printf("Can't open file: %s, err: %v", fileName, err)
		return nil, err
	}
	defer os.Remove(fileName)
	defer file.Close()

	fileBytes, err := ioutil.ReadAll(file)
	if err != nil {
		log.Printf("Can't read file: %s, err: %v", fileName, err)
		return nil, err
	}

	bankResponse := bankResponse{}
	err = json.Unmarshal(fileBytes, &bankResponse)
	if err != nil {
		log.Printf("Can't unmarshal file: %s, err: %v", fileName, err)
		return nil, err
	}

	payments := processTransactions(bankResponse.AccountedTransaction, lastAccountingOrder)

	return payments, nil
}
