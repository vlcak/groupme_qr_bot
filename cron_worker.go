package main

import (
	"fmt"
	"log"
)

func NewCronWorker(bankChecker *BankChecker, sheetOperator *GoogleSheetOperator, messageService *MessageService) *CronWorker {
	return &CronWorker{
		bankChecker:    bankChecker,
		sheetOperator:  sheetOperator,
		messageService: messageService,
	}
}

type CronWorker struct {
	bankChecker    *BankChecker
	sheetOperator  *GoogleSheetOperator
	messageService *MessageService
}

func (cw *CronWorker) CheckNewPayments() {
	log.Printf("Checking new payments")
	payments, err := cw.bankChecker.CheckPayments()
	if err != nil {
		log.Printf("Can't get payments: %v", err)
		return
	}
	accountNumbers, err := cw.sheetOperator.Get("Sheet1!A4:4", "", false)
	if err != nil {
		log.Printf("Can't get Account numbers: %v", err)
		return
	}
	for _, payment := range payments {
		found := false
		for i, account := range accountNumbers {
			if account == payment.AccountNumber {
				cellAddress := fmt.Sprintf("Sheet1!%s2", ToColumnIndex(i))
				v, err := cw.sheetOperator.Get(cellAddress, "FORMULA", false)
				if err != nil {
					log.Printf("Can't get amount cell for payment: %v, %v", payment, err)
				}
				v[0] = fmt.Sprintf("%s+%d", v[0], payment.Amount)
				newValue := []interface{}{v[0]}
				err = cw.sheetOperator.Write(cellAddress, newValue)
				if err != nil {
					log.Printf("Can't store new amount cell for payment: %v, %v", payment, err)
				}
				log.Printf("Added %d to %s, account %s", payment.Amount, payment.Name, payment.AccountNumber)
				found = true
				cw.messageService.SendMessage(fmt.Sprintf("New payment from: %s, account: %s, amount: %d", payment.Name, payment.AccountNumber, payment.Amount), "")
			}
		}
		if !found {
			log.Printf("Payment not matched %v", payment)
			cw.messageService.SendMessage(fmt.Sprintf("Payment not matched! from: %s, account: %s, amount: %d", payment.Name, payment.AccountNumber, payment.Amount), "")
		}
	}
}
