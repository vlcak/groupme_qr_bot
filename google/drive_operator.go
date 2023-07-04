package google

import (
	"context"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
	"log"
)

const (
	LINEUP_TEMPLATE_ID = "1lijx2adeGiokWTpzPKEa_33y9reg3c4J"
	FWD_ROW            = 3
	DEF_ROW            = 16
	GOL_ROW            = 26
	HOME_COL           = 2
	AWAY_COL           = 8
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
