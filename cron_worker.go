package main

import (
	"fmt"
	"log"
	"regexp"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/vlcak/groupme_qr_bot/bank"
	database "github.com/vlcak/groupme_qr_bot/db"
	"github.com/vlcak/groupme_qr_bot/google"
	"github.com/vlcak/groupme_qr_bot/groupme"
	"github.com/vlcak/groupme_qr_bot/tymuj"
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
	exponentialBackoff := backoff.NewExponentialBackOff()
	exponentialBackoff.MaxElapsedTime = 5 * time.Minute
	payments, err := backoff.RetryNotifyWithData(func() ([]bank.Payment, error) {
		return cw.csobClient.CheckPayments()
	}, exponentialBackoff, func(err error, duration time.Duration) {
		log.Printf("Can't get payments: %v, retrying in %s", err, duration)
	})
	if err != nil {
		log.Printf("Can't get payments: %v, trying file", err)
		payments, err = cw.csobClient.CheckPaymentsFromFile()
		if err != nil {
			log.Printf("Can't get payments from file: %v", err)
			return
		}
	}

	userNames, err := cw.sheetOperator.Get("Sheet1!A1:1", "", false)
	if err != nil {
		log.Printf("Can't get user names: %v", err)
		return
	}
	for _, payment := range payments {
		cw.processPayment(payment, userNames)
	}
}

func (cw *CronWorker) CheckUnprocessedPayments() {
	log.Printf("Checking unprocessed payments")
	payments, err := cw.db.GetUnprocessedPayments()
	if err != nil {
		log.Printf("Can't get unprocessed payments: %v", err)
		return
	}

	if len(payments) > 0 {
		log.Printf("Unprocessed payments found - reprocessing: %d", len(payments))
		userNames, err := cw.sheetOperator.Get("Sheet1!A1:1", "", false)
		if err != nil {
			log.Printf("Can't get user names: %v", err)
			return
		}

		for _, payment := range payments {
			log.Printf("Reprocessing payment - name: %s, account: %s, amount: %d", payment.Name.String, payment.Account.String, payment.Amount.Int64)
			cw.processPayment(bank.Payment{
				Name:          payment.Name.String,
				AccountNumber: payment.Account.String,
				Amount:        int(payment.Amount.Int64),
				Order:         int(payment.Order.Int64),
			}, userNames)
		}
	}
}

func (cw *CronWorker) CreateWednesdayEventForPlayers() {
	log.Printf("Creating players event")
	after, _ := time.Parse("2006-01-02", "2023-09-10")
	if after.After(time.Now()) {
		log.Printf("Too early to create event")
		return
	}
	t := time.Now()
	nextWednesday := t.AddDate(0, 0, 7-int(t.Weekday())+3)
	time := "21:00"
	if value, err := cw.db.IsException(nextWednesday.Format("2006-01-02"), time); err != nil {
		log.Printf("Can't check exception: %v", err)
		cw.messageService.SendMessage(fmt.Sprintf("Can't check exception: %v", err), "")
	} else if value {
		log.Printf("Exception for %s %s - NOT SCHEDULING", nextWednesday.Format("2006-01-02"), time)
		cw.messageService.SendMessage(fmt.Sprintf("Exception for %s %s - NOT SCHEDULING", nextWednesday.Format("2006-01-02"), time), "")
		return
	}

	eventCreator := NewEventCreator(cw.tymujClient)
	eventURL, err := eventCreator.CreateEvent("Kateřinky", nextWednesday.Format("2.1."), time, "16", "Hokej 4v4 Kateřinky - hráči", "", false, []int{})
	if err != nil {
		log.Printf("Can't create event: %v", err)
		return
	}
	log.Printf("Event created: %s", eventURL)
	cw.messageService.SendMessage(fmt.Sprintf("Event created: %s", eventURL), "")
}

func (cw *CronWorker) CreateWednesdayEventForGoalies() {
	log.Printf("Creating goalies event")
	after, _ := time.Parse("2006-01-02", "2023-09-10")
	if after.After(time.Now()) {
		log.Printf("Too early to create event")
		return
	}
	t := time.Now()
	nextWednesday := t.AddDate(0, 0, 7-int(t.Weekday())+3)
	time := "21:00"
	if value, err := cw.db.IsException(nextWednesday.Format("2006-01-02"), time); err != nil {
		log.Printf("Can't check exception: %v", err)
		cw.messageService.SendMessage(fmt.Sprintf("Can't check exception: %v", err), "")
	} else if value {
		log.Printf("Exception for %s %s - NOT SCHEDULING", nextWednesday.Format("2006-01-02"), time)
		cw.messageService.SendMessage(fmt.Sprintf("Exception for %s %s - NOT SCHEDULING", nextWednesday.Format("2006-01-02"), time), "")
		return
	}

	eventCreator := NewEventCreator(cw.tymujClient)
	eventURL, err := eventCreator.CreateEvent("Kateřinky", nextWednesday.Format("2.1."), time, "2", "Hokej 4v4 Kateřinky - gólmani", "", false, []int{PLAYERS_GROUP_ID})
	if err != nil {
		log.Printf("Can't create event: %v", err)
		return
	}
	log.Printf("Event created: %s", eventURL)
	cw.messageService.SendMessage(fmt.Sprintf("Event created: %s", eventURL), "")
}

func (cw *CronWorker) processPayment(payment bank.Payment, userNames []string) {
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
				cw.messageService.SendMessage(fmt.Sprintf("Can't store new amount cell for payment: %v, %v", payment, err), "")
				continue
			}
			err = cw.db.MarkPaymentProcessed(payment.Order)
			if err != nil {
				log.Printf("Can't mark payment as processed: %v, %v", payment, err)
				cw.messageService.SendMessage(fmt.Sprintf("Can't mark payment as processed: %v, %v", payment, err), "")
				continue
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
