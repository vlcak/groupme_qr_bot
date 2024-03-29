package tymuj

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	graphql "github.com/hasura/go-graphql-client"
	"github.com/vlcak/groupme_qr_bot/utils"
	"golang.org/x/exp/slices"
	"golang.org/x/oauth2"
)

const (
	BaseURL = "https://app.tymuj.cz/"
	V2URL   = "https://api2.tymuj.cz/graphql"
	RustURL = "https://rust-api.tymuj.cz/graphql"
)

type Atendee struct {
	Id        graphql.ID
	GroupId   graphql.ID
	GroupName string
	Name      string
	RSVP      string
}

type Event struct {
	Id               graphql.ID
	Name             string
	IsPast           bool
	IsGame           bool
	IsAway           bool
	StartTime        time.Time
	EndTime          time.Time
	PlannedTime      time.Time
	AttendanceTime   time.Time
	Capacity         int
	AssignCount      int
	SendReminderDays int
	Location         string
	OpponentName     string
}

func (e *Event) GetURL() string {
	return fmt.Sprintf("%sevents/%s", BaseURL, e.Id)
}

type EventListInput struct {
	TeamId   int    `json:"teamId,omitempty"`
	Upcoming bool   `json:"upcoming,omitempty"`
	Past     bool   `json:"past,omitempty"`
	DateFrom string `json:"dateFrom,omitempty"`
	DateTo   string `json:"dateTo,omitempty"`
}

type Team struct {
	Id      graphql.ID
	Name    string
	Members []Member
}

type Member struct {
	Id     graphql.ID
	UserId graphql.ID
	Name   string
	Karma  int
}

type Location struct {
	Id      graphql.ID
	Name    string
	Address string
}

func (l *Location) Match(location string) bool {
	normalizedLocation := strings.TrimSpace(utils.Normalize(location))
	return strings.Contains(utils.Normalize(l.Name), normalizedLocation) || strings.Contains(utils.Normalize(l.Address), normalizedLocation)
}

type Opponent struct {
	Id   graphql.ID
	Name string
}

func (o *Opponent) Match(opponent string) bool {
	normalizedOpponent := strings.TrimSpace(utils.Normalize(opponent))
	return strings.Contains(utils.Normalize(o.Name), normalizedOpponent)
}

type UserLoginInput struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type EventCreateInput struct {
	TeamId           string           `json:"teamId"`
	IsGame           bool             `json:"isGame"`
	IsAway           bool             `json:"isAway"`
	Name             string           `json:"name,omitempty"`
	Note             string           `json:"note"`
	PlayerIDs        []string         `json:"playerIds"`
	Capacity         int              `json:"capacity,omitempty"`
	LocationID       string           `json:"locationId"`
	OpponentID       *string          `json:"opponentId"`
	SendReminderDays int              `json:"sendReminderDays,omitempty"`
	TimeBlocks       []TimeBlockInput `json:"timeBlocks,omitempty"`
}

type TimeBlockInput struct {
	StartTime      time.Time `json:"startTime,omitempty"`
	EndTime        time.Time `json:"endTime,omitempty"`
	PlannedTime    time.Time `json:"plannedTime,omitempty"`
	AttendanceTime time.Time `json:"attendanceTime,omitempty"`
}

func NewClient(username, password string, teamId int) *Client {
	client := &Client{
		userLogin: UserLoginInput{
			Username: username,
			Password: password,
		},
		teamId:    teamId,
		lastLogin: time.Now().Add(-1 * time.Hour * 24 * 30),
	}
	err := client.createClients()
	if err != nil {
		log.Printf("Unable to create clients: %v", err)
		return nil
	}
	return client
}

type Client struct {
	client2    *graphql.Client
	clientRust *graphql.Client
	teamId     int
	userLogin  UserLoginInput
	lastLogin  time.Time
}

func (c *Client) createClients() error {
	if c.lastLogin.Add(time.Hour * 24).After(time.Now()) {
		log.Printf("Login too soon")
		return errors.New("Login too soon")
	}
	var mutation struct {
		UserLogin struct {
			Tokens struct {
				JWT      string
				Typename string `graphql:"__typename"`
			}
			User struct {
				ID       graphql.ID
				Username string
				Typename string `graphql:"__typename"`
			}
			Typename string `graphql:"__typename"`
		} `graphql:"userLogin(data: $data)"`
	}

	variables := map[string]interface{}{
		"data": c.userLogin,
	}

	loginClient := graphql.NewClient(V2URL, nil)

	if err := loginClient.Mutate(context.Background(), &mutation, variables); err != nil {
		log.Printf("Unable to login: %v", err)
		return err
	}

	token := mutation.UserLogin.Tokens.JWT
	src := oauth2.StaticTokenSource(
		&oauth2.Token{
			AccessToken: token,
			TokenType:   "bearer",
		},
	)
	httpClient := oauth2.NewClient(context.Background(), src)
	c.client2 = graphql.NewClient(V2URL, httpClient)
	c.clientRust = graphql.NewClient(RustURL, httpClient)
	return nil
}

func (c *Client) GetTeam(exceptGroups []int, lowestKarma int) (*Team, error) {
	var query struct {
		Team struct {
			Id      graphql.ID
			Name    string
			Members []struct {
				Id       graphql.ID
				Nickname string
				Karma    int
				User     struct {
					Id          graphql.ID
					Karma       int
					UserProfile struct {
						Id       graphql.ID
						FullName string
						Typename string `graphql:"__typename"`
					}
					Typename string `graphql:"__typename"`
				}
				TeamSubgroup struct {
					Id   graphql.ID
					Name string
				}
				Typename string `graphql:"__typename"`
			}
		} `graphql:"team(teamId: $teamId)"`
	}

	variables := map[string]interface{}{
		"teamId": graphql.ToID(c.teamId),
	}
	if err := c.client2.Query(context.Background(), &query, variables); err != nil {
		if c.createClients() != nil {
			log.Printf("Unable to query team: %v", err)
			return nil, err
		}
		return c.GetTeam(exceptGroups, lowestKarma)
	}
	sort.Ints(exceptGroups)

	team := &Team{
		Id:      query.Team.Id,
		Name:    query.Team.Name,
		Members: []Member{},
	}
	for _, m := range query.Team.Members {
		subgroupId, err := strconv.Atoi(string(m.TeamSubgroup.Id))
		if err != nil {
			log.Printf("Unable to parse group id: %v", err)
			continue
		}
		i := sort.SearchInts(exceptGroups, subgroupId)
		if i < len(exceptGroups) && exceptGroups[i] == subgroupId {
			continue
		}
		member := Member{
			Id:     m.Id,
			UserId: m.User.Id,
			Name:   m.Nickname,
			Karma:  m.Karma,
		}
		if member.Karma == 0 {
			member.Karma = m.User.Karma
		}
		if member.Karma < lowestKarma {
			continue
		}
		if member.Name == "" {
			member.Name = m.User.UserProfile.FullName
		}

		team.Members = append(team.Members, member)
	}

	sort.Slice(team.Members, func(i, j int) bool {
		return team.Members[i].Karma > team.Members[j].Karma
	})
	return team, nil
}

func (c *Client) GetEvents(noGoalies, gamesOnly, past, upcoming bool) ([]Event, error) {
	var query struct {
		Events struct {
			Results []struct {
				Id               graphql.ID
				Name             string
				IsPast           bool
				IsGame           bool
				IsAway           bool
				StartTime        string
				EndTime          string
				PlannedTime      string
				AttendanceTime   string
				Capacity         int
				AssignCount      int
				SendReminderDays int
				Location         struct {
					Id       graphql.ID
					Name     string
					Address  string
					Typename string `graphql:"__typename"`
				}
				Team struct {
					Id       graphql.ID
					Name     string
					Typename string `graphql:"__typename"`
				}
				Opponent struct {
					Id       graphql.ID
					Name     string
					Typename string `graphql:"__typename"`
				}
				Typename string `graphql:"__typename"`
			}
		} `graphql:"events(page: $page, filter: $filter)"`
	}

	var events []Event

	pageNumber := 0
	variables := map[string]interface{}{
		"page": pageNumber,
		"filter": EventListInput{
			TeamId:   c.teamId,
			Upcoming: false,
			Past:     false,
			DateFrom: time.Now().Add(-1 * time.Hour * 24 * 30).Format(time.RFC3339),
			DateTo:   time.Now().Add(time.Hour * 24 * 30).Format(time.RFC3339),
		},
	}
	pageItems := 1
	now := time.Now()
	for pageItems > 0 {
		if err := c.clientRust.Query(context.Background(), &query, variables); err != nil {
			if c.createClients() != nil {
				log.Printf("Unable to query events: %v", err)
				return nil, err
			}
			return c.GetEvents(noGoalies, gamesOnly, past, upcoming)
		}
		pageItems = len(query.Events.Results)
		pageNumber = pageNumber + 1
		variables["page"] = pageNumber
		for _, e := range query.Events.Results {
			if noGoalies && e.Capacity < 4 {
				continue
			}
			if gamesOnly && !e.IsGame {
				continue
			}
			startParsedTime, err := time.Parse(time.RFC3339, e.StartTime)
			if err != nil {
				log.Printf("Unable to parse start time: %v", err)
				continue
			}
			difference := now.Sub(startParsedTime)
			if !upcoming && difference.Seconds() < 0 {
				continue
			}
			if !past && difference.Seconds() > 0 {
				continue
			}
			endParsedTime, _ := time.Parse(time.RFC3339, e.EndTime)
			plannedParsedTime, _ := time.Parse(time.RFC3339, e.PlannedTime)
			attendanceParsedTime, _ := time.Parse(time.RFC3339, e.AttendanceTime)

			name := e.Name
			opponentName := ""
			if e.IsGame {
				opponentName = e.Opponent.Name
				if e.IsAway {
					name = fmt.Sprintf("%s vs %s", e.Opponent.Name, e.Team.Name)
				} else {
					name = fmt.Sprintf("%s vs %s", e.Team.Name, e.Opponent.Name)
				}
			}

			events = append(events, Event{
				Id:               e.Id,
				Name:             name,
				StartTime:        startParsedTime,
				EndTime:          endParsedTime,
				PlannedTime:      plannedParsedTime,
				AttendanceTime:   attendanceParsedTime,
				IsGame:           e.IsGame,
				IsPast:           e.IsPast,
				IsAway:           e.IsAway,
				Capacity:         e.Capacity,
				AssignCount:      e.AssignCount,
				SendReminderDays: e.SendReminderDays,
				Location:         e.Location.Name,
				OpponentName:     opponentName,
			})
		}
		query.Events.Results = nil
	}
	sort.Slice(events, func(i, j int) bool {
		return events[i].StartTime.Unix() > events[j].StartTime.Unix()
	})
	return events, nil
}

func (c *Client) GetAtendees(id graphql.ID, goingOnly bool, exceptGroups []int) ([]Atendee, error) {
	exceptGroupsFilter := []graphql.ID{}
	for _, egi := range exceptGroups {
		exceptGroupsFilter = append(exceptGroupsFilter, graphql.ToID(egi))
	}
	var query struct {
		Event struct {
			Id          graphql.ID
			Name        string
			IsPast      bool
			IsGame      bool
			IsAway      bool
			StartTime   string
			EndTime     string
			Capacity    int
			AssignCount int
			Team        struct {
				Id       graphql.ID
				Name     string
				Typename string `graphql:"__typename"`
			}
			Opponent struct {
				Id       graphql.ID
				Name     string
				Typename string `graphql:"__typename"`
			}
			EventPlayers []struct {
				Answer           string
				EventPlayerGuest struct {
					Id       graphql.ID
					Name     string
					Typename string `graphql:"__typename"`
				}
				Id         graphql.ID
				TeamMember struct {
					Id           graphql.ID
					TeamSubgroup struct {
						Id       graphql.ID
						Name     string
						Typename string `graphql:"__typename"`
					}
					User struct {
						Id          graphql.ID
						UserProfile struct {
							FullName string
							Typename string `graphql:"__typename"`
						}
						Typename string `graphql:"__typename"`
					}
					Typename string `graphql:"__typename"`
				}
				Typename string `graphql:"__typename"`
			}
			Typename string `graphql:"__typename"`
		} `graphql:"event(eventId: $id)"`
	}

	variables := map[string]interface{}{
		"id": id,
	}

	if err := c.clientRust.Query(context.Background(), &query, variables); err != nil {
		if c.createClients() != nil {
			log.Printf("Unable to query atendees: %v", err)
			return nil, err
		}
		return c.GetAtendees(id, goingOnly, exceptGroups)
	}
	var atendees []Atendee
	for _, a := range query.Event.EventPlayers {
		if goingOnly && a.Answer != "GOING" {
			continue
		}
		if slices.Index(exceptGroupsFilter, a.TeamMember.TeamSubgroup.Id) != -1 {
			continue
		}
		if a.TeamMember.User.Id == graphql.ToID(0) {
			atendees = append(atendees, Atendee{
				Id:        a.EventPlayerGuest.Id,
				GroupId:   graphql.ToID(0),
				GroupName: "Guests",
				Name:      a.EventPlayerGuest.Name,
				RSVP:      "GOING",
			})
		} else {
			atendees = append(atendees, Atendee{
				Id:        a.TeamMember.User.Id,
				GroupId:   a.TeamMember.TeamSubgroup.Id,
				GroupName: a.TeamMember.TeamSubgroup.Name,
				Name:      a.TeamMember.User.UserProfile.FullName,
				RSVP:      a.Answer,
			})
		}
	}
	return atendees, nil
}

func (c *Client) GetLocations() ([]Location, error) {
	var query struct {
		EventLocations []struct {
			Id      graphql.ID
			Name    string
			Address string
		} `graphql:"eventLocations(teamId: $teamId)"`
	}

	variables := map[string]interface{}{
		"teamId": graphql.ToID(c.teamId),
	}
	if err := c.client2.Query(context.Background(), &query, variables); err != nil {
		if c.createClients() != nil {
			log.Printf("Unable to query locations: %v", err)
			return nil, err
		}
		return c.GetLocations()
	}

	locations := []Location{}

	for _, l := range query.EventLocations {
		locations = append(locations, Location{
			Id:      l.Id,
			Name:    l.Name,
			Address: l.Address,
		})
	}

	return locations, nil
}

func (c *Client) GetOpponents() ([]Opponent, error) {
	var query struct {
		EventOpponents []struct {
			Id   graphql.ID
			Name string
		} `graphql:"eventOpponents(teamId: $teamId)"`
	}

	variables := map[string]interface{}{
		"teamId": graphql.ToID(c.teamId),
	}
	if err := c.client2.Query(context.Background(), &query, variables); err != nil {
		if c.createClients() != nil {
			log.Printf("Unable to query opponents: %v", err)
			return nil, err
		}
		return c.GetOpponents()
	}

	opponents := []Opponent{}

	for _, o := range query.EventOpponents {
		opponents = append(opponents, Opponent{
			Id:   o.Id,
			Name: o.Name,
		})
	}

	return opponents, nil
}

func (c *Client) CreateEvent(eventRequest EventCreateInput) (*Event, error) {
	var mutation struct {
		CreateEvent []struct {
			Id               graphql.ID
			Name             string
			IsPast           bool
			IsGame           bool
			IsAway           bool
			StartTime        string
			EndTime          string
			PlannedTime      string
			AttendanceTime   string
			Capacity         int
			AssignCount      int
			SendReminderDays int
			Team             struct {
				Id       graphql.ID
				Name     string
				Typename string `graphql:"__typename"`
			}
			Opponent struct {
				Id       graphql.ID
				Name     string
				Typename string `graphql:"__typename"`
			}
			Location struct {
				Id       graphql.ID
				Name     string
				Address  string
				Typename string `graphql:"__typename"`
			}
			Typename string `graphql:"__typename"`
		} `graphql:"createEvent(data: $data)"`
	}

	variables := map[string]interface{}{
		"data": eventRequest,
	}

	if err := c.client2.Mutate(context.Background(), &mutation, variables); err != nil {
		if c.createClients() != nil {
			log.Printf("Unable to create event: %v", err)
			return nil, err
		}
		return c.CreateEvent(eventRequest)
	}

	if len(mutation.CreateEvent) != 1 {
		log.Printf("Events not created: %v", mutation.CreateEvent)
		return nil, errors.New("Events not created")
	}

	newEvent := mutation.CreateEvent[0]

	startParsedTime, err := time.Parse(time.RFC3339, newEvent.StartTime)
	if err != nil {
		log.Printf("Unable to parse start time: %v", err)
	}
	endParsedTime, _ := time.Parse(time.RFC3339, newEvent.EndTime)
	plannedParsedTime, _ := time.Parse(time.RFC3339, newEvent.PlannedTime)
	attendanceParsedTime, _ := time.Parse(time.RFC3339, newEvent.AttendanceTime)

	name := newEvent.Name
	opponentName := ""
	if newEvent.IsGame {
		opponentName = newEvent.Opponent.Name
		if newEvent.IsAway {
			name = fmt.Sprintf("%s vs %s", newEvent.Opponent.Name, newEvent.Team.Name)
		} else {
			name = fmt.Sprintf("%s vs %s", newEvent.Team.Name, newEvent.Opponent.Name)
		}
	}

	return &Event{
		Id:               newEvent.Id,
		Name:             name,
		IsPast:           newEvent.IsPast,
		IsGame:           newEvent.IsGame,
		IsAway:           newEvent.IsAway,
		StartTime:        startParsedTime,
		EndTime:          endParsedTime,
		PlannedTime:      plannedParsedTime,
		AttendanceTime:   attendanceParsedTime,
		Capacity:         newEvent.Capacity,
		AssignCount:      newEvent.AssignCount,
		SendReminderDays: newEvent.SendReminderDays,
		Location:         newEvent.Location.Name,
		OpponentName:     opponentName,
	}, nil
}

func print(v interface{}) {
	w := json.NewEncoder(os.Stdout)
	w.SetIndent("", "\t")
	err := w.Encode(v)
	if err != nil {
		panic(err)
	}
}
