package main

import (
	"image/color"

	qrcode "github.com/skip2/go-qrcode"
)

const (
	LEVEL = qrcode.Medium
	SIZE  = 250
)

func NewQRPaymentGenerator() *QRPaymentGenerator {
	return &QRPaymentGenerator{}
}

type QRPaymentGenerator struct {
}

func (qpg *QRPaymentGenerator) Generate(amount, split int) ([]byte, error) {
	content := "BLABLA"
	var q *qrcode.QRCode

	q, err := qrcode.New(content, LEVEL)

	if err != nil {
		return nil, err
	}
	q.ForegroundColor = color.Black
	q.BackgroundColor = color.White

	return q.PNG(SIZE)
}
