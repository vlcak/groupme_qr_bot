package google

import (
	"context"
	"log"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

const (
	LINEUP_TEMPLATE_ID = "1NXmU55pK5NMAmQ6naDyb3HRFcPRxnQ2NKPJGBopvZp8"
	FWD_ROW            = 3
	DEF_ROW            = 14
	GOL_ROW            = 21
	HOME_COL           = 1
	AWAY_COL           = 7
)

func NewDriveOperator(ctx context.Context) (*DriveOperator, error) {
	srv, err := drive.NewService(ctx, option.WithCredentialsFile(CREDENTIALS_FILE_PATH))
	if err != nil {
		log.Printf("Unable to retrieve Drive client: %v", err)
		return nil, err
	}
	return &DriveOperator{
		service: srv,
	}, nil
}

type DriveOperator struct {
	service *drive.Service
}

func (do *DriveOperator) CopyFile(fileId, name string) (*drive.File, error) {
	return do.service.Files.Copy(fileId, &drive.File{Name: name}).Do()
}

func (do *DriveOperator) ListFiles() (*drive.FileList, error) {
	return do.service.Files.List().Do()
}
