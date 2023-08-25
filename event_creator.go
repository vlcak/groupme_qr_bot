package main

import (
	"github.com/vlcak/groupme_qr_bot/tymuj"

	"errors"
	"fmt"
	"log"
	"strconv"
	"time"
)

func NewEventCreator(tymujClient *tymuj.Client) *EventCreator {
	return &EventCreator{
		tymujClient: tymujClient,
	}
}

type EventCreator struct {
	tymujClient *tymuj.Client
}

func (ec *EventCreator) CreateEvent(where, date, startTime, capacity, name, oponent string, away bool, exceptGroups []int) (string, error) {
	log.Printf("Creating event: where: %s, date: %s, time: %s, capacity: %s, name: %s, opponent %s\n", where, date, startTime, capacity, name, oponent)
	team, err := ec.tymujClient.GetTeam(exceptGroups, 0)
	if err != nil {
		log.Printf("Unable to get team: %v\n", err)
		return "", err
	}
	playerIDs := []string{}
	for _, player := range team.Members {
		playerIDs = append(playerIDs, string(player.UserId))
	}

	eventCreateInput := tymuj.EventCreateInput{
		TeamId:           string(team.Id),
		IsGame:           oponent != "",
		Note:             "",
		SendReminderDays: 3,
		PlayerIDs:        playerIDs,
	}

	// parse where
	location, err := ec.tymujClient.GetLocations()
	if err != nil {
		log.Printf("Unable to get locations: %v\n", err)
		return "", err
	}
	if len(location) == 0 {
		log.Printf("No locations found\n")
		return "", errors.New("No locations found")
	}
	for _, loc := range location {
		if loc.Match(where) {
			eventCreateInput.LocationID = string(loc.Id)
			break
		}
	}
	if eventCreateInput.LocationID == "" {
		log.Printf("No location found\n")
		return "", errors.New("No location found")
	}

	if eventCreateInput.IsGame {
		// parse oponent
		opponents, err := ec.tymujClient.GetOpponents()
		if err != nil {
			log.Printf("Unable to get opponents: %v\n", err)
			return "", err
		}
		if len(opponents) == 0 {
			log.Printf("No opponents found\n")
			return "", errors.New("No opponents found")
		}
		for _, opp := range opponents {
			if opp.Match(oponent) {
				oID := string(opp.Id)
				eventCreateInput.OpponentID = &oID
				break
			}
		}
		if eventCreateInput.OpponentID == nil {
			log.Printf("No opponent found\n")
			return "", errors.New("No opponent found")
		}
		eventCreateInput.IsAway = away
	} else {
		eventCreateInput.Name = name
	}

	// parse when
	locationPrague, err := time.LoadLocation("Europe/Prague")
	if err != nil {
		log.Printf("Error:", err)
		return "", err
	}

	now := time.Now()
	t, err := time.ParseInLocation("2006-2.1. 15:04", fmt.Sprintf("%d-%s %s", now.Year(), date, startTime), locationPrague)
	if err != nil {
		log.Printf("Unable to parse date: %v\n", err)
		return "", err
	}
	log.Printf("Parsed date: %s\n", t)
	if t.Before(now) {
		t = t.AddDate(1, 0, 0)
	}
	// t = t.Local()
	length := 60
	if eventCreateInput.IsGame {
		length = 75
	}
	timeBlock := tymuj.TimeBlockInput{
		StartTime:      t,
		EndTime:        t.Add(time.Minute * time.Duration(length)),
		PlannedTime:    t.Add(time.Minute * -30),
		AttendanceTime: t.Add(time.Hour * -48),
	}
	eventCreateInput.TimeBlocks = []tymuj.TimeBlockInput{timeBlock}

	// parse capacity
	capacityInt, err := strconv.Atoi(capacity)
	if err != nil {
		log.Printf("Unable to parse capacity: %v\n", err)
		return "", err
	}
	eventCreateInput.Capacity = capacityInt

	log.Printf("Create event input: %+v\n", eventCreateInput)

	// create event
	event, err := ec.tymujClient.CreateEvent(eventCreateInput)
	if err != nil {
		log.Printf("Unable to create event: %v\n", err)
		return "", err
	}
	log.Printf("Created event: %+v\n", event)

	return event.GetURL(), nil
}
