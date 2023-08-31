package bank

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/cookiejar"
	"time"

	database "github.com/vlcak/groupme_qr_bot/db"
)

func NewCsobClient(accountNumber int, bankURL string, db *database.Client) *CsobClient {
	return &CsobClient{
		accountNumber: accountNumber,
		url:           bankURL,
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

func (cc *CsobClient) GetAccountURL() string {
	return cc.viewURL
}

type account struct {
	AccountNumberM24 int `json:"accountNumberM24"`
}

type filter struct {
	Name      string   `json:"name"`
	Operator  string   `json:"operator"`
	ValueList []string `json:"valueList"`
}

type paging struct {
	PageNumber  int `json:"pageNumber"`
	RowsPerPage int `json:"rowsPerPage"`
}

type sorting struct {
	Direction string `json:"direction"`
	Name      string `json:"name"`
	Order     int    `json:"order"`
}

type requestPayments struct {
	AccountList []account `json:"accountList"`
	FilterList  []filter  `json:"filterList"`
	Paging      paging    `json:"paging"`
	SortList    []sorting `json:"sortList"`
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
		AccountingDate  int64
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
	jar, err := cookiejar.New(nil)
	if err != nil {
		log.Printf("Can't create cookie jar: %v", err)
		return nil, err
	}
	client := &http.Client{
		Jar:     jar,
		Timeout: 30 * time.Second,
	}
	r, err := http.NewRequest("GET", cc.viewURL, nil)
	if err != nil {
		log.Printf("Can't create bank request %v\n", err)
		return nil, err
	}
	r.Header.Add("Accept", "*/*")
	r.Header.Add("Accept-Encoding", "gzip, deflate, br")
	r.Header.Add("Accept-Language", "cs")
	r.Header.Add("Connection", "keep-alive")
	r.Header.Add("Sec-Fetch-Dest", "document")
	r.Header.Add("Sec-Fetch-Mode", "navigate")
	r.Header.Add("Sec-Fetch-Site", "none")
	r.Header.Add("Sec-Fetch-User", "?1")
	r.Header.Add("sec-ch-ua", "\"Chromium\";v=\"116\", \"Not)A;Brand\";v=\"24\", \"Microsoft Edge\";v=\"116\"")
	r.Header.Add("sec-ch-ua-mobile", "?0")
	r.Header.Add("sec-ch-ua-platform", "\"Windows\"")
	r.Header.Add("Upgrade-Insecure-Requests", "1")
	r.Header.Add("User-Agent", "Mozilla/5.0 (X11; Linux x86_64; rv:94.0) Gecko/20100101 Firefox/94.0")
	response, err := client.Do(r)
	if err != nil {
		log.Printf("Error sending bank request: %v\n", err)
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		log.Printf("Unexpected bank request return code: %d\n", response.StatusCode)
		log.Printf("Response: %v", response)
		return nil, errors.New("unexpected bank request return code")
	}

	payload := &requestPayments{
		AccountList: []account{
			{
				AccountNumberM24: cc.accountNumber,
			},
		},
		FilterList: []filter{},
		Paging: paging{
			RowsPerPage: 20,
			PageNumber:  1,
		},
		SortList: []sorting{
			{
				Direction: "DESC",
				Name:      "AccountingOrder",
				Order:     1,
			},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Can't marshal bank account request payload: %v", err)
		return nil, err
	}

	r, err = http.NewRequest("POST", cc.url, bytes.NewReader(body))
	if err != nil {
		log.Printf("Can't create bank request %v\n", err)
		return nil, err
	}
	r.Header.Add("Accept", "application/json, text/plain, */*")
	r.Header.Add("Content-Type", "application/json")
	r.Header.Add("Referer", fmt.Sprintf("https://www.csob.cz/portal/firmy/bezne-ucty/transparentni-ucty/ucet?account=%d)", cc.accountNumber))
	r.Header.Add("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/113.0.0.0 Safari/537.36 Edg/113.0.1774.35")
	response, err = client.Do(r)
	if err != nil {
		log.Printf("Error sending bank request: %v\n", err)
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		log.Printf("Unexpected bank request return code: %d\n", response.StatusCode)
		log.Printf("Response: %v", response)
		return nil, errors.New("unexpected bank request return code")
	}

	body, err = ioutil.ReadAll(response.Body)
	if err != nil {
		log.Printf("Can't read bank response body: %v\n", err)
		return nil, err
	}

	bankResponse := bankResponse{}

	err = json.Unmarshal(body, &bankResponse)
	if err != nil {
		log.Printf("Can't unmarshal bank response body: %v\nbody: %v", err, string(body))
		return nil, err
	}
	// print(bankResponse)
	var payments []Payment
	hitLast := false
	for _, transaction := range bankResponse.AccountedTransaction {
		if transaction.BaseInfo.AccountingOrder > lastAccountingOrder && transaction.BaseInfo.AccountAmountData.Amount > 0 {
			payments = append(payments, Payment{
				Name: transaction.TransactionTypeChoice.DomesticPayment.PartyName,
				AccountNumber: fmt.Sprintf(
					"%d/%s",
					transaction.TransactionTypeChoice.DomesticPayment.PartyAccount.DomesticAccount.AccountNumber,
					transaction.TransactionTypeChoice.DomesticPayment.PartyAccount.DomesticAccount.BankCode,
				),
				Message:   transaction.TransactionTypeChoice.DomesticPayment.Message.Message1,
				Amount:    transaction.BaseInfo.AccountAmountData.Amount,
				Order:     transaction.BaseInfo.AccountingOrder,
				Timestamp: time.Unix(transaction.BaseInfo.AccountingDate/1000, 0),
			})
		}
		if transaction.BaseInfo.AccountingOrder <= lastAccountingOrder {
			hitLast = true
			break
		}
	}
	if !hitLast {
		log.Printf("Not all payments checked!")
	}

	return payments, nil
}
