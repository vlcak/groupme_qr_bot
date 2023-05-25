package google

import (
	"context"
	"fmt"
	"log"

	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

const (
	IDO_INSERT_ROWS       = "INSERT_ROWS"
	VIO_USER_ENTERED      = "USER_ENTERED"
	VRO_FORMULA           = "FORMULA"
	VRO_UNFORMATTED_VALUE = "UNFORMATTED_VALUE"
)

func NewSheetOperator(ctx context.Context, spreadsheetId, credentialsFilePath string) (*SheetOperator, error) {
	srv, err := sheets.NewService(ctx, option.WithCredentialsFile(credentialsFilePath))
	if err != nil {
		log.Printf("Unable to retrieve Sheets client: %v", err)
		return nil, err
	}
	return &SheetOperator{
		spreadsheetId: spreadsheetId,
		service:       srv,
	}, nil
}

type SheetOperator struct {
	spreadsheetId string
	service       *sheets.Service
}

func ToColumnIndex(index int) string {
	if index < 26 {
		return string('A' + index)
	}
	return fmt.Sprintf("%s%s", string('A'+(index/26)-1), string('A'+(index%26)))
}

func (so *SheetOperator) Get(getRange, valueRenderOption string, removeEmpty bool) ([]string, error) {
	if valueRenderOption == "" {
		valueRenderOption = VRO_UNFORMATTED_VALUE
	}
	resp, err := so.service.Spreadsheets.Values.Get(so.spreadsheetId, getRange).ValueRenderOption(valueRenderOption).Do()
	if err != nil {
		log.Printf("Unable to retrieve data from sheet: %v", err)
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

func (so *SheetOperator) Write(writeRange string, newValues []interface{}) error {
	valueInputOption := VIO_USER_ENTERED
	values := [][]interface{}{newValues}

	rb := &sheets.ValueRange{
		Values: values,
	}
	response, err := so.service.Spreadsheets.Values.Update(so.spreadsheetId, writeRange, rb).ValueInputOption(valueInputOption).Do()
	if err != nil || response.HTTPStatusCode != 200 {
		log.Printf("Unable to write cell: %v", err)
		return err
	}
	return nil
}

func (so *SheetOperator) AppendLine(sheetName string, newValues []interface{}) error {
	valueInputOption := VIO_USER_ENTERED
	insertDataOption := IDO_INSERT_ROWS
	values := [][]interface{}{newValues}

	rb := &sheets.ValueRange{
		Values: values,
	}
	response, err := so.service.Spreadsheets.Values.Append(so.spreadsheetId, sheetName, rb).ValueInputOption(valueInputOption).InsertDataOption(insertDataOption).Do()
	if err != nil || response.HTTPStatusCode != 200 {
		log.Printf("Unable to insert new row: %v", err)
		return err
	}
	return nil
}
