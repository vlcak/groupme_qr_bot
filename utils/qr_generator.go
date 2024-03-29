package utils

import (
	"image/color"
	"log"
	"regexp"

	qrcode "github.com/skip2/go-qrcode"
)

const (
	LEVEL              = qrcode.Medium
	SIZE               = 250
	SPLITTER           = "*"
	TYPE               = "SPD"
	VERSION            = "1.0"
	ACCOUNT            = "ACC:"
	AMOUNT             = "AM:"
	CURRENCY           = "CC:"
	MESSAGE            = "MSG:"
	MESSAGE_MAX_LENGTH = 60
)

func NewQRPaymentGenerator() *QRPaymentGenerator {
	return &QRPaymentGenerator{
		messageRegexp: regexp.MustCompile("[^A-Za-z0-9$%+-./: ]"),
	}
}

type QRPaymentGenerator struct {
	messageRegexp *regexp.Regexp
}

func (qpg *QRPaymentGenerator) Generate(message, account, amount string) ([]byte, error) {
	content := TYPE + SPLITTER + VERSION + SPLITTER
	// add account
	content = content + ACCOUNT + account + SPLITTER
	// add amount
	content = content + AMOUNT + amount + SPLITTER
	// add message
	escapedMessage := qpg.messageRegexp.ReplaceAllLiteralString(message, "")
	if len(escapedMessage) > MESSAGE_MAX_LENGTH {
		escapedMessage = escapedMessage[:MESSAGE_MAX_LENGTH]
	}
	content = content + MESSAGE + escapedMessage

	log.Printf("QR content: %s\n", content)
	var q *qrcode.QRCode

	q, err := qrcode.New(content, LEVEL)

	if err != nil {
		return nil, err
	}
	q.ForegroundColor = color.Black
	q.BackgroundColor = color.White

	return q.PNG(SIZE)
}
