package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

type GroupmeMessage struct {
	Attachments []interface{} `json:"attachments"`
	AvatarUrl   string        `json:"avatar_url"`
	CreatedAt   int64         `json:"created_at"`
	GroupId     string        `json:"group_id"`
	Id          string        `json:"id"`
	Name        string        `json:"name"`
	SenderId    string        `json:"sender_id"`
	SenderType  string        `json:"sender_type"`
	SourceGuid  string        `json:"source_guid"`
	System      bool          `json:"system"`
	Text        string        `json:"text"`
	UserId      string        `json:"user_id"`
}

func NewMessageProcessor(imageService *ImageService, messageService *MessageService, selfID string) *MessageProcessor {
	accounts := map[string]string{}
	m := &MessageProcessor{
		imageService:     imageService,
		messageService:   messageService,
		paymentGenerator: NewQRPaymentGenerator(),
		selfID:           selfID,
		accounts:         accounts,
	}
	return m
}

type MessageProcessor struct {
	imageService     *ImageService
	messageService   *MessageService
	paymentGenerator *QRPaymentGenerator
	selfID           string
	accounts         map[string]string
}

func (mp *MessageProcessor) ProcessMessage(body io.ReadCloser) error {
	m := GroupmeMessage{}
	if err := json.NewDecoder(body).Decode(&m); err != nil {
		fmt.Printf("ERROR: %v\n", err)
		return err
	}
	// Ignore own messages
	if m.SenderId == mp.selfID {
		fmt.Printf("Ignoring own message\n")
		return nil
	}
	fmt.Printf("Message text: %s ID %s \n", m.Text, m.SenderId)

	parsedMessage := strings.SplitAfterN(m.Text, " ", 4)
	switch command := strings.TrimSpace(parsedMessage[0]); command {
	case "PAY":
		if len(parsedMessage) != 4 {
			fmt.Printf("Wrong PAY format\n")
			return nil
		}
		mp.createPayment(m.SenderId, strings.TrimSpace(parsedMessage[1]), strings.TrimSpace(parsedMessage[2]), parsedMessage[3])
	case "ADD_ACCOUNT":
		if len(parsedMessage) != 2 {
			fmt.Printf("Wrong ADD_ACCOUNT format\n")
			return nil
		}
		mp.accounts[m.SenderId] = strings.TrimSpace(parsedMessage[1])
	default:
		fmt.Printf("Not a command\n")
	}

	return nil
}

func (mp *MessageProcessor) createPayment(senderId, amoutStr, splitStr, message string) error {
	accountNumber := mp.accounts[senderId]
	if accountNumber == "" {
		fmt.Printf("Unknown sender\n")
		mp.messageService.SendMessage("I don't know your account", "")
		return errors.New("Unknown sender")
	}

	amount, err := strconv.Atoi(amoutStr)
	if err != nil {
		fmt.Printf("Cant parse amount %v\n", err)
		return err
	}

	split, err := strconv.Atoi(splitStr)
	if err != nil {
		fmt.Printf("Cant parse split %v\n", err)
		return err
	}

	image, err := mp.paymentGenerator.Generate(message, accountNumber, amount, split)
	if err != nil {
		fmt.Printf("Error generating QR %v\n", err)
		return err
	}
	imageURL, err := mp.imageService.Upload(image)
	if err != nil {
		fmt.Printf("Error during image upload %v\n", err)
		return err
	}
	mp.messageService.SendMessage("Hello from BOT!", imageURL)
	return nil
}
