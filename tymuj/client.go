package tymuj

import (
	"context"
	"encoding/json"
	"fmt"
	graphql "github.com/hasura/go-graphql-client"
	"golang.org/x/exp/slices"
	"golang.org/x/oauth2"
	"log"
	"os"
	"sort"
	"time"
)

type Atendee struct {
	Id        graphql.ID
	GroupId   graphql.ID
	GroupName string
	Name      string
	RSVP      string
}

type Event struct {
	Id          graphql.ID
	Name        string
	IsPast      bool
	IsGame      bool
	StartTime   time.Time
	Capacity    int
	AssignCount int
}

type EventListInput struct {
	teamId   graphql.ID
	upcoming bool
	past     bool
	dateFrom time.Time
	dateTo   time.Time
}

func NewClient(token string, teamId int) *Client {
	src := oauth2.StaticTokenSource(
		&oauth2.Token{
			AccessToken: token,
			TokenType:   "bearer",
		},
	)
	httpClient := oauth2.NewClient(context.Background(), src)
	return &Client{
		client: graphql.NewClient("https://rust-api.tymuj.cz/graphql", httpClient),
		teamId: teamId,
	}
}

type Client struct {
	client *graphql.Client
	teamId int
}

func (c *Client) GetEvents(noGoalies, pastOnly bool) ([]Event, error) {
	var query struct {
		Events struct {
			Results []struct {
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
				Typename string `graphql:"__typename"`
			}
		} `graphql:"events(page: $page, filter: $filter)"`
	}

	var events []Event

	pageNumber := 0
	variables := map[string]interface{}{
		"page": pageNumber,
		"filter": EventListInput{
			teamId:   graphql.ToID(c.teamId),
			upcoming: false,
			past:     true,
			dateFrom: time.Now().Add(-1 * time.Hour * 24),
			dateTo:   time.Now(),
		},
	}
	pageItems := 1
	now := time.Now()
	for pageItems > 0 {
		if err := c.client.Query(context.Background(), &query, variables); err != nil {
			log.Printf("Unable to query events: %v", err)
			return nil, err
		}
		pageItems = len(query.Events.Results)
		pageNumber = pageNumber + 1
		variables["page"] = pageNumber
		for _, e := range query.Events.Results {
			if noGoalies && e.Capacity < 4 {
				continue
			}
			parsedTime, err := time.Parse(time.RFC3339, e.StartTime)
			if err != nil {
				log.Printf("Unable to parse time: %v", err)
				continue
			}
			difference := now.Sub(parsedTime)
			if pastOnly && difference.Seconds() < 0 {
				continue
			}

			name := e.Name
			if e.IsGame {
				if e.IsAway {
					name = fmt.Sprintf("%s vs %s", e.Opponent.Name, e.Team.Name)
				} else {
					name = fmt.Sprintf("%s vs %s", e.Team.Name, e.Opponent.Name)
				}
			}

			events = append(events, Event{
				Id:          e.Id,
				Name:        name,
				StartTime:   parsedTime,
				IsGame:      e.IsGame,
				IsPast:      e.IsPast,
				Capacity:    e.Capacity,
				AssignCount: e.AssignCount,
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

	if err := c.client.Query(context.Background(), &query, variables); err != nil {
		log.Printf("Unable to query atendees: %v", err)
		return nil, err
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

func print(v interface{}) {
	w := json.NewEncoder(os.Stdout)
	w.SetIndent("", "\t")
	err := w.Encode(v)
	if err != nil {
		panic(err)
	}
}
