package main

import (
	"fmt"
	"log"
	"regexp"
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
	userNames, err := cw.sheetOperator.Get("Sheet1!A1:1", "", false)
	if err != nil {
		log.Printf("Can't get user names: %v", err)
		return
	}
	for _, payment := range payments {
		found := false
		resent, err := regexp.MatchString(`^TO \d{9,10}/\d{4,4}`, payment.Message)
		if err != nil {
			log.Printf("Can't check payment message: %s, err: %v", payment.Message, err)
		}
		if resent {
			payment.AccountNumber = payment.Message[3:]
		}
		for i, account := range accountNumbers {
			if account == payment.AccountNumber || (account == "hosts" && !found) {
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
				log.Printf("Added %d to %s(%s), account %s, resent: %t", payment.Amount, payment.Name, userNames[i], payment.AccountNumber, resent)
				found = (account != "hosts")
				cw.messageService.SendMessage(fmt.Sprintf("New payment from: %s(%s), account: %s, amount: %d, resent: %t", payment.Name, userNames[i], payment.AccountNumber, payment.Amount, resent), "")
				break
			}
		}
		if !found {
			log.Printf("Payment not matched and added to hosts %v", payment)
			cw.messageService.SendMessage(fmt.Sprintf("Payment not matched, adding to hosts! from: %s, account: %s, amount: %d, resent: %t", payment.Name, payment.AccountNumber, payment.Amount, resent), "")
		}
	}
}
