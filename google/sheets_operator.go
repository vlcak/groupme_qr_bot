package google

import (
	"context"
	"fmt"
	"log"

	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

const (
	CREDENTIALS_FILE_PATH = "sa_credentials.json"

	IDO_INSERT_ROWS       = "INSERT_ROWS"
	VIO_USER_ENTERED      = "USER_ENTERED"
	VRO_FORMULA           = "FORMULA"
	VRO_UNFORMATTED_VALUE = "UNFORMATTED_VALUE"

	HOSTS = "hosté"
)

func NewSheetOperator(ctx context.Context, spreadsheetId string) (*SheetOperator, error) {
	srv, err := sheets.NewService(ctx, option.WithCredentialsFile(CREDENTIALS_FILE_PATH))
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

func (so *SheetOperator) GetReadOnlyURL() string {
	return fmt.Sprintf("https://docs.google.com/spreadsheets/d/%s/htmlview", so.spreadsheetId)
}

func (so *SheetOperator) GetReadOnlyURLToSheet(id int) string {
	ids, err := so.GetSheetIDs()
	if err != nil {
		log.Printf("Unable to retrieve data from sheet: %v", err)
		return so.GetReadOnlyURL()
	}
	if id < 0 || id >= len(ids) {
		log.Printf("Invalid sheet index: %d", id)
		return so.GetReadOnlyURL()
	}
	return fmt.Sprintf("https://docs.google.com/spreadsheets/d/%s/htmlview?gid=%d", so.spreadsheetId, ids[id])
}

func (so *SheetOperator) GetSheetIDs() ([]int, error) {
	sheet, err := so.service.Spreadsheets.Get(so.spreadsheetId).Do()
	if err != nil {
		log.Printf("Unable to retrieve data from sheet: %v", err)
		return nil, err
	}
	sheetIDs := []int{}
	for _, s := range sheet.Sheets {
		sheetIDs = append(sheetIDs, int(s.Properties.SheetId))
	}
	return sheetIDs, nil
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
