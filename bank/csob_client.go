package bank

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	// "github.com/vlcak/groupme_qr_bot/db"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
)

func NewCsobClient(accountNumber int, bankURL string) *CsobClient {
	return &CsobClient{
		accountNumber: accountNumber,
		url:           bankURL,
		filePath:      "lastAccountingOrder.txt",
	}
}

type CsobClient struct {
	accountNumber int
	url           string
	filePath      string
}

type Payment struct {
	Name          string
	AccountNumber string
	Message       string
	Amount        int
}

func (cc *CsobClient) CheckPayments() ([]Payment, error) {
	previousLastAccountingOrder, err := readLastAccountingOrder(cc.filePath)
	if err != nil {
		log.Printf("Can't get last accounting order: %v", err)
		previousLastAccountingOrder = 400
	}
	payments, newLastAccountingOrder, err := cc.paymentsSinceLastCheck(previousLastAccountingOrder)
	if err != nil {
		log.Printf("Can't get payments: %v", err)
		return nil, err
	}
	// Store lastAccountingOrder
	err = saveLastAccountingOrder(cc.filePath, newLastAccountingOrder)
	if err != nil {
		log.Printf("Can't store last accounting order: %v", err)
		return nil, err
	}

	return payments, nil
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

func (cc *CsobClient) paymentsSinceLastCheck(lastAccountingOrder int) ([]Payment, int, error) {
	payload := &requestPayments{
		AccountList: []account{
			account{
				AccountNumberM24: cc.accountNumber,
			},
		},
		FilterList: []filter{},
		Paging: paging{
			RowsPerPage: 10,
			PageNumber:  1,
		},
		SortList: []sorting{
			sorting{
				Direction: "DESC",
				Name:      "AccountingOrder",
				Order:     1,
			},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Can't marshal bank account request payload: %v", err)
		return nil, 0, err
	}

	r, err := http.NewRequest("POST", cc.url, bytes.NewReader(body))
	if err != nil {
		log.Printf("Can't create bank request %v\n", err)
		return nil, 0, err
	}
	r.Header.Add("Accept", "application/json, text/plain, */*")
	r.Header.Add("Content-Type", "application/json")
	r.Header.Add("Referer", fmt.Sprintf("https://www.csob.cz/portal/firmy/bezne-ucty/transparentni-ucty/ucet?account=%d)", cc.accountNumber))
	r.Header.Add("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/113.0.0.0 Safari/537.36 Edg/113.0.1774.35")
	client := &http.Client{}
	response, err := client.Do(r)
	if err != nil {
		log.Printf("Error sending bank request: %v\n", err)
		return nil, 0, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		log.Printf("Unexpected bank request return code: %d\n", response.StatusCode)
		log.Printf("Response: %v", response)
		return nil, 0, errors.New("unexpected bank request return code")
	}

	body, err = ioutil.ReadAll(response.Body)
	if err != nil {
		log.Printf("Can't read bank response body: %v\n", err)
		return nil, 0, err
	}

	bankResponse := bankResponse{}

	err = json.Unmarshal(body, &bankResponse)
	if err != nil {
		log.Printf("Can't unmarshal bank response body: %v\nbody: %v", err, string(body))
		return nil, 0, err
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
				Message: transaction.TransactionTypeChoice.DomesticPayment.Message.Message1,
				Amount:  transaction.BaseInfo.AccountAmountData.Amount,
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

	return payments, bankResponse.AccountedTransaction[0].BaseInfo.AccountingOrder, nil
}

// Retrieves a lastAccountingOrder from a local file.
func readLastAccountingOrder(file string) (int, error) {
	f, err := os.Open(file)
	if err != nil {
		log.Printf("Unable to open lastAccountingOrder for reading: %v", err)
		return 0, err
	}
	defer f.Close()

	r := bufio.NewReader(f)
	line, _, err := r.ReadLine()
	if err != nil {
		log.Printf("Unable to read lastAccountingOrder: %v", err)
		return 0, err
	}

	return strconv.Atoi(string(line))
}

// Saves lastAccountingOrder to a file path.
func saveLastAccountingOrder(path string, lastAccountOrderNumber int) error {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Printf("Unable to open lastAccountingOrder for writing: %v", err)
		return err
	}
	defer f.Close()
	if _, err = f.WriteString(strconv.Itoa(lastAccountOrderNumber)); err != nil {
		log.Printf("Unable to write lastAccountingOrder: %v", err)
		return err
	}
	return nil
}
