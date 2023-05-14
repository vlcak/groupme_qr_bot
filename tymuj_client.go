package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"time"
	"encoding/json"
	"golang.org/x/exp/slices"
	"golang.org/x/oauth2"
	graphql "github.com/hasura/go-graphql-client"
)

type TymujAtendee struct {
	Id graphql.ID
	GroupId graphql.ID
	GroupName string
	Name string
	RSVP string
}

type TymujEvent struct {
	Id graphql.ID
	Name string
	IsPast bool
	IsGame bool
	StartTime time.Time
	Capacity int
	AssignCount int
}

type EventListInput struct {
	teamId graphql.ID
	upcoming bool
	past bool
	dateFrom time.Time
	dateTo time.Time
}

func NewTymujClient(token string, teamId int) *TymujClient {
	src := oauth2.StaticTokenSource(
		&oauth2.Token{
			AccessToken: token,
			TokenType: "bearer",
		},
	)
	httpClient := oauth2.NewClient(context.Background(), src)
	return &TymujClient{
		client: graphql.NewClient("https://rust-api.tymuj.cz/graphql", httpClient),
		teamId: teamId,
	}
}

type TymujClient struct {
	client *graphql.Client
	teamId int
}

func (tc *TymujClient) GetEvents(noGoalies, pastOnly bool) ([]TymujEvent, error) {
	var query struct {
		Events struct {
			Results [] struct {
				Id graphql.ID
				Name string
				IsPast bool
				IsGame bool
				IsAway bool
				StartTime string
				EndTime string
				Capacity int
				AssignCount int
				Team struct {
					Id graphql.ID
					Name string
					Typename string `graphql:"__typename"`
				}
				Opponent struct {
					Id graphql.ID
					Name string
					Typename string `graphql:"__typename"`
				}
				Typename string `graphql:"__typename"`
			}
		} `graphql:"events(page: $page, filter: $filter)"`
	}

	var events []TymujEvent

	pageNumber := 0
	variables := map[string]interface{}{
		"page": pageNumber,
		"filter": EventListInput{
			teamId: graphql.ToID(tc.teamId),
			upcoming: false,
			past: true,
			dateFrom: time.Now().Add(-1 * time.Hour * 24),
			dateTo: time.Now(),
		},
	}
	pageItems := 1
	now := time.Now()
	// now, _ := time.Parse(time.RFC3339, "2023-05-10T19:59:58+02:00")
	for pageItems > 0 {
		if err := tc.client.Query(context.Background(), &query, variables); err != nil {
			print(err)
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
				fmt.Println(err)
				continue
			}
			difference := now.Sub(parsedTime)
			if pastOnly && difference.Seconds() < 0 {
				continue
			}

			name := e.Name
			if (e.IsGame) {
				if (e.IsAway) {
					name = fmt.Sprintf("%s vs %s", e.Opponent.Name, e.Team.Name)
				} else {
					name = fmt.Sprintf("%s vs %s", e.Team.Name, e.Opponent.Name)
				}
			}

			events = append(events, TymujEvent{
				Id: e.Id,
				Name: name,
				StartTime: parsedTime,
				IsGame: e.IsGame,
				IsPast: e.IsPast,
				Capacity: e.Capacity,
				AssignCount: e.AssignCount,
			})
		}
		query.Events.Results = nil
	}
	sort.Slice(events, func(i, j int) bool {
		return events[i].StartTime.Unix() > events[j].StartTime.Unix()
	})
	print(events[0])
	return events, nil
}

func (tc *TymujClient) GetAtendees(id graphql.ID, goingOnly bool, exceptGroups []int) ([]TymujAtendee, error) {
	exceptGroupsFilter := []graphql.ID{}
	for _, egi := range(exceptGroups) {
		exceptGroupsFilter = append(exceptGroupsFilter, graphql.ToID(egi))
	}
	var query struct {
		Event struct {
			Id graphql.ID
			Name string
			IsPast bool
			IsGame bool
			IsAway bool
			StartTime string
			EndTime string
			Capacity int
			AssignCount int
			Team struct {
				Id graphql.ID
				Name string
				Typename string `graphql:"__typename"`
			}
			Opponent struct {
				Id graphql.ID
				Name string
				Typename string `graphql:"__typename"`
			}
			EventPlayers []struct {
				Answer string
				// EventPlayerGuest bool
				Id graphql.ID
				TeamMember struct {
					Id graphql.ID
					TeamSubgroup struct {
						Id graphql.ID
						Name string
						Typename string `graphql:"__typename"`
					}
					User struct {
						Id graphql.ID
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

	if err := tc.client.Query(context.Background(), &query, variables); err != nil {
		print(err)
		return nil, err
	}
	var atendees []TymujAtendee
	for _, a := range(query.Event.EventPlayers) {
		if goingOnly && a.Answer != "GOING" {
			continue
		}
		if slices.Index(exceptGroupsFilter, a.TeamMember.TeamSubgroup.Id) != -1 {
			continue
		}
		atendees = append(atendees, TymujAtendee{
			Id: a.TeamMember.User.Id,
			GroupId: a.TeamMember.TeamSubgroup.Id,
			GroupName: a.TeamMember.TeamSubgroup.Name,
			Name: a.TeamMember.User.UserProfile.FullName,
			RSVP: a.Answer,
		})
	}
	print(atendees)
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
