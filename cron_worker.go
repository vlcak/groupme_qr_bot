package main

import (
	"fmt"
	"github.com/vlcak/groupme_qr_bot/bank"
	"github.com/vlcak/groupme_qr_bot/db"
	"github.com/vlcak/groupme_qr_bot/google"
	"github.com/vlcak/groupme_qr_bot/groupme"
	"github.com/vlcak/groupme_qr_bot/tymuj"
	"log"
	"regexp"
	"time"
)

func NewCronWorker(csobClient *bank.CsobClient, sheetOperator *google.SheetOperator, tymujClient *tymuj.Client, messageService *groupme.MessageService, db *database.Client) *CronWorker {
	return &CronWorker{
		csobClient:     csobClient,
		sheetOperator:  sheetOperator,
		tymujClient:    tymujClient,
		messageService: messageService,
		db:             db,
	}
}

type CronWorker struct {
	csobClient     *bank.CsobClient
	sheetOperator  *google.SheetOperator
	tymujClient    *tymuj.Client
	messageService *groupme.MessageService
	db             *database.Client
}

func (cw *CronWorker) CheckNewPayments() {
	log.Printf("Checking new payments")
	payments, err := cw.csobClient.CheckPayments()
	if err != nil {
		log.Printf("Can't get payments: %v", err)
		return
	}

	userNames, err := cw.sheetOperator.Get("Sheet1!A1:1", "", false)
	if err != nil {
		log.Printf("Can't get user names: %v", err)
		return
	}
	for _, payment := range payments {
		resent, err := regexp.MatchString(`^TO \d{9,10}/\d{4,4}`, payment.Message)
		if err != nil {
			log.Printf("Can't check payment message: %s, err: %v", payment.Message, err)
		}
		if resent {
			payment.AccountNumber = payment.Message[3:]
		}

		userName, err := cw.db.GetName(payment.AccountNumber)
		if err != nil {
			log.Printf("Can't get user name for account: %s, err: %v", payment.AccountNumber, err)
			userName = google.HOSTS
		}

		for i, name := range userNames {
			if name == userName {
				cellAddress := fmt.Sprintf("Sheet1!%s2", google.ToColumnIndex(i))
				v, err := cw.sheetOperator.Get(cellAddress, google.VRO_FORMULA, false)
				if err != nil {
					log.Printf("Can't get amount cell for payment: %v, %v", payment, err)
				}

				v[0] = fmt.Sprintf("%s+%d", v[0], payment.Amount)
				newValue := []interface{}{v[0]}
				err = cw.sheetOperator.Write(cellAddress, newValue)
				if err != nil {
					log.Printf("Can't store new amount cell for payment: %v, %v", payment, err)
				}

				log.Printf("Added %d to %s(%s), account %s, order: %d, resent: %t", payment.Amount, payment.Name, name, payment.AccountNumber, payment.Order, resent)
				if name == google.HOSTS {
					log.Printf("Payment not matched and added to hosts %v", payment)
				}

				cw.messageService.SendMessage(
					fmt.Sprintf(
						"New payment from: %s(%s), account: %s, amount: %d, order: %d, resent: %t",
						payment.Name,
						name,
						payment.AccountNumber,
						payment.Amount,
						payment.Order,
						resent),
					"")
				break
			}
		}
	}
}

func (cw *CronWorker) CreateEvent() {
	log.Printf("Creating event")
	t := time.Now()
	nextWednesday := t.AddDate(0, 0, 7-int(t.Weekday())+3)

	eventCreator := NewEventCreator(cw.tymujClient)
	eventURL, err := eventCreator.CreateEvent("Říčany", nextWednesday.Format("2.1."), "21:00", "12", "Hokej 3v3 Říčany", "", false, []int{GOALIES_GROUP_ID})
	if err != nil {
		log.Printf("Can't create event: %v", err)
		return
	}
	log.Printf("Event created: %s", eventURL)
	cw.messageService.SendMessage(fmt.Sprintf("Event created: %s", eventURL), "")
}
