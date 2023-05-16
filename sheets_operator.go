package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

func NewGoogleSheetOperator(ctx context.Context, spreadsheetId, credentialsFilePath string) (*GoogleSheetOperator, error) {
	b, err := os.ReadFile(credentialsFilePath)
	if err != nil {
		log.Fatalf("Unable to read client secret file: %v", err)
		return nil, err
	}

	// If modifying these scopes, delete your previously saved token.json.
	config, err := google.ConfigFromJSON(b, "https://www.googleapis.com/auth/spreadsheets")
	if err != nil {
		log.Fatalf("Unable to parse client secret file to config: %v", err)
		return nil, err
	}
	client := getClient(config)

	srv, err := sheets.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		log.Fatalf("Unable to retrieve Sheets client: %v", err)
		return nil, err
	}
	return &GoogleSheetOperator{
		spreadsheetId: spreadsheetId,
		service:       srv,
	}, nil
}

type GoogleSheetOperator struct {
	spreadsheetId string
	service       *sheets.Service
}

// Retrieve a token, saves the token, then returns the generated client.
func getClient(config *oauth2.Config) *http.Client {
	// The file token.json stores the user's access and refresh tokens, and is
	// created automatically when the authorization flow completes for the first
	// time.
	tokFile := "token.json"
	tok, err := tokenFromFile(tokFile)
	if err != nil {
		tok = getTokenFromWeb(config)
		saveToken(tokFile, tok)
	}
	return config.Client(context.Background(), tok)
}

// Request a token from the web, then returns the retrieved token.
func getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Go to the following link in your browser then type the "+
		"authorization code: \n%v\n", authURL)

	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		log.Fatalf("Unable to read authorization code: %v", err)
	}

	tok, err := config.Exchange(context.TODO(), authCode)
	if err != nil {
		log.Fatalf("Unable to retrieve token from web: %v", err)
	}
	return tok
}

// Retrieves a token from a local file.
func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tok := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(tok)
	return tok, err
}

// Saves a token to a file path.
func saveToken(path string, token *oauth2.Token) {
	fmt.Printf("Saving credential file to: %s\n", path)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalf("Unable to cache oauth token: %v", err)
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
}

func (gso *GoogleSheetOperator) Get(getRange string, removeEmpty bool) ([]string, error) {
	resp, err := gso.service.Spreadsheets.Values.Get(gso.spreadsheetId, getRange).Do()
	if err != nil {
		log.Fatalf("Unable to retrieve data from sheet: %v", err)
		return nil, err
	}

	var values []string
	if len(resp.Values) == 0 {
		log.Printf("No data found.")
		return values, nil
	}
	for _, row := range resp.Values {
		for _, cell := range row {
			val := fmt.Sprintf("%v", cell)
			if !removeEmpty || val != "" {
				values = append(values, val)
			}
		}
	}

	return values, nil
}

func (gso *GoogleSheetOperator) Write(writeRange string, newValues []interface{}) error {
	valueInputOption := "USER_ENTERED"
	insertDataOption := "INSERT_ROWS"
	values := [][]interface{}{newValues}

	rb := &sheets.ValueRange{
		Values: values,
	}
	response, err := gso.service.Spreadsheets.Values.Update(gso.spreadsheetId, writeRange, rb).ValueInputOption(valueInputOption).Do()
	if err != nil || response.HTTPStatusCode != 200 {
		log.Fatalf("Unable to write cell: %v", err)
		return err
	}
	return nil
}

func (gso *GoogleSheetOperator) AppendLine(sheetName string, newValues []interface{}) error {
	valueInputOption := "USER_ENTERED"
	insertDataOption := "INSERT_ROWS"
	values := [][]interface{}{newValues}

	rb := &sheets.ValueRange{
		Values: values,
	}
	response, err := gso.service.Spreadsheets.Values.Append(gso.spreadsheetId, sheetName, rb).ValueInputOption(valueInputOption).InsertDataOption(insertDataOption).Do()
	if err != nil || response.HTTPStatusCode != 200 {
		log.Fatalf("Unable to insert new row: %v", err)
		return err
	}
	return nil
}
