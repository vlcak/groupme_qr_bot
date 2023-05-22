package main

import (
	"context"
	"fmt"
	"log"

	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

func NewGoogleSheetOperator(ctx context.Context, spreadsheetId, credentialsFilePath string) (*GoogleSheetOperator, error) {
	srv, err := sheets.NewService(ctx, option.WithCredentialsFile(credentialsFilePath))
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

func ToColumnIndex(index int) string {
	if index < 26 {
		return string('A' + index)
	}
	return fmt.Sprintf("%s%s", string('A'+(index/26)-1), string('A'+(index%26)))
}

func (gso *GoogleSheetOperator) Get(getRange, valueRenderOption string, removeEmpty bool) ([]string, error) {
	if valueRenderOption == "" {
		valueRenderOption = "UNFORMATTED_VALUE"
	}
	resp, err := gso.service.Spreadsheets.Values.Get(gso.spreadsheetId, getRange).ValueRenderOption(valueRenderOption).Do()
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
