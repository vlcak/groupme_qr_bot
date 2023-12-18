package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/adrg/strutil"
	"github.com/adrg/strutil/metrics"
	database "github.com/vlcak/groupme_qr_bot/db"
	"github.com/vlcak/groupme_qr_bot/google"
	"github.com/vlcak/groupme_qr_bot/groupme"
	"github.com/vlcak/groupme_qr_bot/tymuj"
	"github.com/vlcak/groupme_qr_bot/utils"
	"golang.org/x/exp/slices"
)

const (
	GOALIES_GROUP_ID = 2662
	PLAYERS_GROUP_ID = 2663
	TEAM_NAME        = "B-Tým"
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
	driveOperator *google.DriveOperator,
	selfID string,
	db *database.Client,
) *MessageProcessor {
	m := &MessageProcessor{
		imageService:     imageService,
		messageService:   messageService,
		tymujClient:      tymujClient,
		sheetOperator:    sheetOperator,
		driveOperator:    driveOperator,
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
	driveOperator    *google.DriveOperator
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
			mp.messageService.SendMessage("Wrong QR format", "")
			return nil
		}
		err := mp.createPayment(m.SenderId, strings.TrimSpace(parsedMessage[1]), strings.TrimSpace(parsedMessage[2]), parsedMessage[3])
		if err != nil {
			mp.messageService.SendMessage(fmt.Sprintf("Error occured when processing QR: %v", err), "")
		}
	case "PAY":
		if len(parsedMessage) < 2 || len(parsedMessage) > 3 {
			log.Printf("Wrong PAY format\n")
			mp.messageService.SendMessage("Wrong PAY format", "")
			return nil
		}
		userAmount := ""
		if len(parsedMessage) == 3 {
			userAmount = strings.TrimSpace(parsedMessage[2])
		}
		err := mp.processEvent(m.SenderId, strings.TrimSpace(parsedMessage[1]), userAmount)
		if err != nil {
			mp.messageService.SendMessage(fmt.Sprintf("Error occured when processing PAY: %v", err), "")
		}
	case "ADD_ACCOUNT":
		if len(parsedMessage) != 2 {
			log.Printf("Wrong ADD_ACCOUNT format\n")
			mp.messageService.SendMessage("Wrong ADD_ACCOUNT format", "")
			return nil
		}
		err := mp.db.SetGroupmeAccount(m.SenderId, strings.TrimSpace(parsedMessage[1]))
		if err != nil {
			mp.messageService.SendMessage(fmt.Sprintf("Error occured when processing ADD_ACCOUNT: %v", err), "")
		}
	case "LINEUP":
		if len(parsedMessage) < 1 {
			log.Printf("Wrong LINEUP format\n")
			mp.messageService.SendMessage("Wrong LINEUP format", "")
			return nil
		}
		err := mp.processLineup(strings.Replace(strings.Join(parsedMessage[1:], " "), "  ", " ", -1))
		if err != nil {
			mp.messageService.SendMessage(fmt.Sprintf("Error occured when processing LINEUP: %v", err), "")
		}
	case "CREATE_GAMES":
		if len(parsedMessage) != 2 {
			log.Printf("Wrong CREATE_GAMES format\n")
			mp.messageService.SendMessage("Wrong CREATE_GAMES format", "")
			return nil
		}
		err := mp.createGames(strings.TrimSpace(parsedMessage[1]))
		if err != nil {
			mp.messageService.SendMessage(fmt.Sprintf("Error occured when processing CREATE_GAMES: %v", err), "")
		}
	case "SCHEDULE_EXCEPTION":
		if len(parsedMessage) < 2 || len(parsedMessage) > 3 {
			log.Printf("Wrong SCHEDULE_EXCEPTION format\n")
			mp.messageService.SendMessage("Wrong SCHEDULE_EXCEPTION format", "")
			return nil
		}
		time := ""
		if len(parsedMessage) == 3 {
			time = strings.TrimSpace(parsedMessage[2])
		}
		err := mp.ScheduleException(strings.TrimSpace(parsedMessage[1]), time)
		if err != nil {
			mp.messageService.SendMessage(fmt.Sprintf("Error occured when processing SCHEDULE_EXCEPTION: %v", err), "")
		}
	case "HELP":
		mp.messageService.SendMessage("Commands:\n"+
			"QR <amount> <split> <description> - creates QR code for payment\n"+
			"PAY <amount> ?<perUser> - processes latest event\n"+
			"ADD_ACCOUNT <account> - adds bank account to groupme account\n"+
			"LINEUP - creates lineup for next game\n"+
			"CREATE_GAMES <date> - creates games from given spreadsheet\n"+
			"SCHEDULE_EXCEPTION <date> ?<time> - unschedule game\n"+
			"HELP - prints this message", "")
	default:
		log.Printf("Not a command\n")
		mp.messageService.SendMessage(fmt.Sprintf("Not a command: %s", command), "")
	}

	return nil
}

func (mp *MessageProcessor) processEvent(senderId, amoutStr, perUserAmount string) error {
	events, err := mp.tymujClient.GetEvents(true, false, true, false)
	if err != nil {
		log.Printf("Unable to get events: %v\n", err)
		return err
	}
	lastEvent := events[0]
	log.Printf("Last event: %v", lastEvent)

	tymujAtendees, err := mp.tymujClient.GetAtendees(lastEvent.Id, true, []int{GOALIES_GROUP_ID})
	if err != nil {
		log.Printf("Unable to get atendees: %v\n", err)
		return err
	}
	var atendees []string
	for _, a := range tymujAtendees {
		atendees = append(atendees, utils.Normalize(a.Name))
	}

	accountNumber, err := mp.db.GetGroupmeAccount(senderId)
	if err != nil || accountNumber == "" {
		log.Printf("Unknown sender\n")
		mp.messageService.SendMessage("I don't know your account", "")
		return errors.New("unknown sender")
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
	// split := len(atendees)
	// amountSplitted := (amount + split - 1) / split
	amountSplitted := 250
	if perUserAmount != "" {
		amountSplitted, err = strconv.Atoi(perUserAmount)
		if err != nil {
			log.Printf("Cant parse per user amount %v\n", err)
			return err
		}
	} else if lastEvent.IsGame || lastEvent.Capacity > 12 {
		amountSplitted = 300
	}

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
	sheetNames := make([]string, len(originalSheetNames)-1)
	copy(sheetNames, originalSheetNames)
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
	accountNumber, err := mp.db.GetGroupmeAccount(senderId)
	if err != nil || accountNumber == "" {
		log.Printf("Unknown sender\n")
		mp.messageService.SendMessage("I don't know your account", "")
		return errors.New("unknown sender")
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

func (mp *MessageProcessor) processLineup(captain string) error {
	events, err := mp.tymujClient.GetEvents(false, true, false, true)
	if err != nil {
		log.Printf("Unable to get next game: %v\n", err)
		return err
	}
	if len(events) == 0 {
		log.Printf("No future games found\n")
		return errors.New("no future games found")
	}

	// get oldest event
	lastEvent := events[len(events)-1]
	log.Printf("Last event: %v", lastEvent)
	// get lineup
	atendees, err := mp.tymujClient.GetAtendees(lastEvent.Id, true, []int{})
	if err != nil {
		log.Printf("Unable to get atendees: %v\n", err)
		return err
	}
	// get posts and numbers
	players := []database.Player{}
	notProcessed := []string{}

	for _, atendee := range atendees {
		// get player
		name := strings.Replace(strings.TrimSpace(atendee.Name), "  ", " ", -1)
		player, err := mp.db.GetPlayerByName(name)
		if err != nil {
			notProcessed = append(notProcessed, atendee.Name)
			log.Printf("Unable to get player: %s, err:%v\n", atendee.Name, err)
			continue
		}
		players = append(players, player)
	}

	sort.Slice(players, func(i, j int) bool {
		if !players[i].Number.Valid && !players[j].Number.Valid {
			return players[i].Name.String < players[j].Name.String
		} else if !players[i].Number.Valid {
			return false
		} else if !players[j].Number.Valid {
			return true
		}
		return players[i].Number.Int64 < players[j].Number.Int64
	})

	unknownPosts := []string{}
	lineupFileName := fmt.Sprintf("%s - %s", lastEvent.StartTime.Format("20060102"), lastEvent.Name)
	newLineup, err := mp.driveOperator.CopyFile(google.LINEUP_TEMPLATE_ID, lineupFileName)
	if err != nil {
		log.Printf("Unable to copy lineup template: %v\n", err)
		return err
	}

	column := google.HOME_COL
	opponentColumn := google.AWAY_COL
	if lastEvent.IsAway {
		column = google.AWAY_COL
		opponentColumn = google.HOME_COL
	}
	fwdIndex := google.FWD_ROW
	defIndex := google.DEF_ROW
	golIndex := google.GOL_ROW
	i := 0
	sheetOperator, err := google.NewSheetOperator(context.Background(), newLineup.Id)
	if err != nil {
		log.Printf("Unable to create sheet operator: %v\n", err)
		return err
	}

	bTeamNameAddress := fmt.Sprintf("Sheet1!%s%d", google.ToColumnIndex(column-1), 1)
	err = sheetOperator.Write(bTeamNameAddress, []interface{}{strings.ToUpper(TEAM_NAME)})
	if err != nil {
		log.Printf("Unable to write to sheet: %v\n", err)
		return err
	}
	opponentTeamNameAddress := fmt.Sprintf("Sheet1!%s%d", google.ToColumnIndex(opponentColumn-1), 1)
	err = sheetOperator.Write(opponentTeamNameAddress, []interface{}{strings.ToUpper(lastEvent.OpponentName)})
	if err != nil {
		log.Printf("Unable to write to sheet: %v\n", err)
		return err
	}

	forward := ""
	defense := ""
	goalie := ""
	captainAssigned := false
	for _, player := range players {
		name := player.Name.String
		if player.Name.String == captain {
			captainAssigned = true
			name = fmt.Sprintf("%s (C)", name)
		}
		switch player.Post.String {
		case database.FORWARD:
			i = fwdIndex
			forward += fmt.Sprintf("%s %d\n", name, player.Number.Int64)
			fwdIndex++
		case database.DEFENSE:
			i = defIndex
			defense += fmt.Sprintf("%s %d\n", name, player.Number.Int64)
			defIndex++
		case database.GOALIE:
			i = golIndex
			goalie += fmt.Sprintf("%s %d\n", name, player.Number.Int64)
			golIndex++
		default:
			log.Printf("Unknown post: %s\n", player.Post.String)
			unknownPosts = append(unknownPosts, name)
			continue
		}

		cellAddress := fmt.Sprintf("Sheet1!%s%d", google.ToColumnIndex(column), i)
		record := []interface{}{name, player.Number.Int64}
		err = sheetOperator.Write(cellAddress, record)
		if err != nil {
			log.Printf("Unable to write to sheet: %v\n", err)
			return err
		}
	}

	mp.messageService.SendMessage(
		fmt.Sprintf(
			"%s game\nFORWARD:\n%s\nDEFENSE:\n%s\nGOALIE:\n%s",
			lastEvent.Name,
			forward,
			defense,
			goalie), "")

	if len(notProcessed) > 0 {
		mp.messageService.SendMessage(
			fmt.Sprintf(
				"Players not processed: \n%s",
				strings.Join(notProcessed, ", ")), "")
	}

	if len(unknownPosts) > 0 {
		mp.messageService.SendMessage(
			fmt.Sprintf(
				"Unknown posts: \n%s",
				strings.Join(unknownPosts, ", ")), "")
	}

	if !captainAssigned {
		mp.messageService.SendMessage(
			fmt.Sprintf(
				"Captain not assigned: %s",
				captain), "")
	}

	mp.messageService.SendMessage(
		fmt.Sprintf(
			"Lineup sheet URL: %s",
			sheetOperator.GetReadOnlyURL()), "")

	return nil
}

func (mp *MessageProcessor) createGames(sheetURL string) error {
	googleSheetOperator, err := google.NewSheetOperator(context.Background(), sheetURL)
	if err != nil {
		log.Printf("Unable to create sheet operator: %v\n", err)
		return err
	}

	rowIndex := 1
	row, err := googleSheetOperator.Get(fmt.Sprintf("Sheet1!A%d:%s%d", rowIndex, google.ToColumnIndex((5)), rowIndex), google.VRO_FORMATTED_VALUE, false)
	for err == nil && len(row) > 0 && row[0] != "" {
		log.Printf("GETTING: Sheet1!A%d:%s%d\n", rowIndex, google.ToColumnIndex(5), rowIndex)
		if len(row) != 6 {
			log.Printf("Invalid row length: %d\n", len(row))
			return errors.New("invalid row length")
		}
		isAway := false
		opponent := row[1]
		if utils.Normalize(row[0]) != utils.Normalize(TEAM_NAME) {
			isAway = true
			opponent = row[0]
		}
		date := row[2]
		startTime := row[3]
		capacity := row[4]
		where := row[5]

		eventCreator := NewEventCreator(mp.tymujClient)
		eventURL, err := eventCreator.CreateEvent(where, date, startTime, capacity, "", opponent, isAway, []int{})
		if err != nil {
			log.Printf("Unable to create event: %v\n", err)
			return err
		}
		mp.messageService.SendMessage(
			fmt.Sprintf(
				"Event created: %s",
				eventURL), "")
		rowIndex++
		row, err = googleSheetOperator.Get(fmt.Sprintf("Sheet1!A%d:%s%d", rowIndex, google.ToColumnIndex((5)), rowIndex), google.VRO_FORMATTED_VALUE, false)
	}
	if err != nil {
		log.Printf("Unable to read row: %v\n", err)
		return err
	}
	return nil
}

func (mp *MessageProcessor) ScheduleException(edate, etime string) error {
	_, err := time.Parse("2006-01-02", edate)
	if err != nil {
		log.Printf("Unable to parse date: %v\n", err)
		mp.messageService.SendMessage(
			fmt.Sprintf(
				"Unable to parse date: %s",
				edate), "")
		return err
	}
	if etime != "" {
		_, err := time.Parse("15:04", etime)
		if err != nil {
			log.Printf("Unable to parse time: %v\n", err)
			mp.messageService.SendMessage(
				fmt.Sprintf(
					"Unable to parse time: %s",
					etime), "")
			return err
		}
	}

	err = mp.db.StoreScheduleException(edate, etime)
	if err != nil {
		log.Printf("Unable to store schedule exception: %v\n", err)
		return err
	}
	mp.messageService.SendMessage(
		fmt.Sprintf(
			"Exception stored: %s %s",
			edate,
			etime), "")
	return nil
}
