package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"strconv"
	"strings"

	"github.com/adrg/strutil"
	"github.com/adrg/strutil/metrics"
	"github.com/vlcak/groupme_qr_bot/db"
	"github.com/vlcak/groupme_qr_bot/google"
	"github.com/vlcak/groupme_qr_bot/groupme"
	"github.com/vlcak/groupme_qr_bot/tymuj"
	"github.com/vlcak/groupme_qr_bot/utils"
	"golang.org/x/exp/slices"
)

const (
	GOALIES_GROUP_ID = 2662
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

func NewMessageProcessor(
	imageService *groupme.ImageService,
	messageService *groupme.MessageService,
	tymujClient *tymuj.Client,
	sheetOperator *google.SheetOperator,
	selfID string,
	db *database.Client,
) *MessageProcessor {

	m := &MessageProcessor{
		imageService:     imageService,
		messageService:   messageService,
		tymujClient:      tymujClient,
		sheetOperator:    sheetOperator,
		paymentGenerator: utils.NewQRPaymentGenerator(),
		selfID:           selfID,
		db:               db,
	}
	return m
}

type MessageProcessor struct {
	imageService     *groupme.ImageService
	messageService   *groupme.MessageService
	paymentGenerator *utils.QRPaymentGenerator
	sheetOperator    *google.SheetOperator
	tymujClient      *tymuj.Client
	selfID           string
	db               *database.Client
}

func (mp *MessageProcessor) ProcessMessage(body io.ReadCloser) error {
	m := GroupmeMessage{}
	if err := json.NewDecoder(body).Decode(&m); err != nil {
		log.Printf("ERROR: %v\n", err)
		return err
	}
	// Ignore own messages
	if m.SenderId == mp.selfID {
		log.Printf("Ignoring own message\n")
		return nil
	}
	log.Printf("Message text: %s ID %s \n", m.Text, m.SenderId)

	parsedMessage := strings.SplitAfterN(m.Text, " ", 4)
	switch command := strings.TrimSpace(parsedMessage[0]); command {
	case "QR":
		if len(parsedMessage) != 4 {
			log.Printf("Wrong QR format\n")
			return nil
		}
		err := mp.createPayment(m.SenderId, strings.TrimSpace(parsedMessage[1]), strings.TrimSpace(parsedMessage[2]), parsedMessage[3])
		if err != nil {
			mp.messageService.SendMessage(fmt.Sprintf("Error occured when processing QR: %v", err), "")
		}
	case "PAY":
		if len(parsedMessage) != 2 {
			log.Printf("Wrong PAY format\n")
			return nil
		}
		err := mp.processEvent(m.SenderId, strings.TrimSpace(parsedMessage[1]))
		if err != nil {
			mp.messageService.SendMessage(fmt.Sprintf("Error occured when processing PAY: %v", err), "")
		}
	case "ADD_ACCOUNT":
		if len(parsedMessage) != 2 {
			log.Printf("Wrong ADD_ACCOUNT format\n")
			return nil
		}
		err := mp.db.SetAccount(m.SenderId, strings.TrimSpace(parsedMessage[1]))
		if err != nil {
			mp.messageService.SendMessage(fmt.Sprintf("Error occured when processing ADD_ACCOUNT: %v", err), "")
		}
	default:
		log.Printf("Not a command\n")
		mp.messageService.SendMessage(fmt.Sprintf("Not a command: %s", command), "")
	}

	return nil
}

func (mp *MessageProcessor) processEvent(senderId, amoutStr string) error {
	events, err := mp.tymujClient.GetEvents(true, true)
	if err != nil {
		log.Fatalf("Unable to get events: %v\n", err)
		return err
	}
	lastEvent := events[0]
	log.Printf("Last event: %v", lastEvent)

	tymujAtendees, err := mp.tymujClient.GetAtendees(lastEvent.Id, true, []int{GOALIES_GROUP_ID})
	if err != nil {
		log.Fatalf("Unable to get atendees: %v\n", err)
		return err
	}
	var atendees []string
	for _, a := range tymujAtendees {
		atendees = append(atendees, utils.Normalize(a.Name))
	}

	accountNumber, err := mp.db.GetAccount(senderId)
	if err != nil || accountNumber == "" {
		log.Printf("Unknown sender\n")
		mp.messageService.SendMessage("I don't know your account", "")
		return errors.New("Unknown sender")
	}

	amount, err := strconv.Atoi(amoutStr)
	if err != nil {
		log.Printf("Cant parse amount %v\n", err)
		return err
	}

	eventName := "hokej"
	if lastEvent.IsGame {
		eventName = "zapas"
	}
	message := fmt.Sprintf("%s %s", eventName, lastEvent.StartTime.Format("2.1."))
	split := len(atendees)
	amountSplitted := (amount + split - 1) / split

	image, err := mp.paymentGenerator.Generate(message, accountNumber, strconv.Itoa(amountSplitted))
	if err != nil {
		log.Printf("Error generating QR %v\n", err)
		return err
	}
	imageURL, err := mp.imageService.Upload(image)
	if err != nil {
		log.Printf("Error during image upload %v\n", err)
		return err
	}
	mp.messageService.SendMessage(fmt.Sprintf("Here is the payment QR for %d, msg: %s:", amountSplitted, message), imageURL)

	originalSheetNames, err := mp.sheetOperator.Get("Sheet1!D1:1", "", true)
	if err != nil {
		log.Printf("Can't get sheet names %v\n", err)
		return err
	}
	// remove hosts & normalize
	sheetNames := originalSheetNames[:len(originalSheetNames)-1]
	utils.NormalizeArray(sheetNames)
	remainings, err := mp.sheetOperator.Get("Sheet1!D3:3", "", true)
	if err != nil {
		log.Printf("Can't get sheet remainings %v\n", err)
		return err
	}
	var sufficient, insufficient []string

	row := []interface{}{message, amount, amountSplitted}
	var processed []string
	lev := metrics.NewLevenshtein()
	for i, name := range sheetNames {
		pos := slices.IndexFunc(atendees, func(aName string) bool {
			return strutil.Similarity(aName, name, lev) > 0.75
		})
		if pos != -1 {
			log.Printf("ASSIGNED: %s:%s, val: %f\n", name, atendees[pos], strutil.Similarity(atendees[pos], name, lev))
			processed = append(processed, atendees[pos])
			atendees = append(atendees[:pos], atendees[pos+1:]...)
			row = append(row, "1")
			rem, err := strconv.Atoi(remainings[i])
			if err != nil {
				log.Printf("Can't parse %s to int %v\n", remainings[i], err)
				continue
			}
			if rem >= amountSplitted {
				sufficient = append(sufficient, originalSheetNames[i])
			} else {
				insufficient = append(insufficient, originalSheetNames[i])
			}
		} else {
			row = append(row, "")
		}
	}
	// the rest are hosts
	if len(atendees) > 0 {
		row = append(row, len(atendees))
		row = append(row, strings.Join(atendees, ","))
		insufficient = append(insufficient, atendees...)
	}
	err = mp.sheetOperator.AppendLine("Sheet1", row)
	if err != nil {
		log.Printf("Can't insert row %v\n", err)
		return err
	}

	mp.messageService.SendMessage(
		fmt.Sprintf(
			"Processed %d atendees, hosts: %s\nBalance OK: %d, BAD: %d:",
			len(processed),
			strings.Join(atendees, ","),
			len(sufficient),
			len(insufficient)),
		"")
	mp.messageService.SendMessage(
		fmt.Sprintf(
			"Platba pro: %s",
			strings.Join(insufficient, ",")),
		"")
	return nil
}

func (mp *MessageProcessor) createPayment(senderId, amoutStr, splitStr, message string) error {
	accountNumber, err := mp.db.GetAccount(senderId)
	if err != nil || accountNumber == "" {
		log.Printf("Unknown sender\n")
		mp.messageService.SendMessage("I don't know your account", "")
		return errors.New("Unknown sender")
	}

	amount, err := strconv.Atoi(amoutStr)
	if err != nil {
		log.Printf("Cant parse amount %v\n", err)
		return err
	}

	split, err := strconv.Atoi(splitStr)
	if err != nil {
		log.Printf("Cant parse split %v\n", err)
		return err
	}

	amountSplitted := strconv.Itoa((amount + split - 1) / split)

	image, err := mp.paymentGenerator.Generate(message, accountNumber, amountSplitted)
	if err != nil {
		log.Printf("Error generating QR %v\n", err)
		return err
	}
	imageURL, err := mp.imageService.Upload(image)
	if err != nil {
		log.Printf("Error during image upload %v\n", err)
		return err
	}
	mp.messageService.SendMessage(fmt.Sprintf("Here is the payment QR for %s, msg: %s:", amountSplitted, message), imageURL)
	return nil
}
