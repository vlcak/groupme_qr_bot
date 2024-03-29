package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/newrelic/go-agent/v3/newrelic"
	"github.com/robfig/cron"
	"github.com/vlcak/groupme_qr_bot/bank"
	database "github.com/vlcak/groupme_qr_bot/db"
	"github.com/vlcak/groupme_qr_bot/google"
	"github.com/vlcak/groupme_qr_bot/groupme"
	"github.com/vlcak/groupme_qr_bot/tymuj"
)

var (
	flagBotToken        = flag.String("bot-token", "", "Bot TOKEN")
	flagBotID           = flag.String("bot-id", "", "Bot ID")
	flagPort            = flag.String("port", ":80", "Service address (e.g. :80)")
	flagUserToken       = flag.String("user-token", "", "User token for images")
	flagDbURL           = flag.String("db", "", "Database URL")
	flagTymujLogin      = flag.String("tymuj-login", "", "Tymuj login")
	flagTymujPassword   = flag.String("tymuj-password", "", "Tymuj password")
	flagTymujTeamID     = flag.Int("tymuj-team-id", 33489, "Tymuj team ID")
	flagGoogleSheetID   = flag.String("google-sheet-id", "", "Google sheet ID")
	flagAccountNumber   = flag.Int("account-number", 311396620, "Account number")
	flagNewRelicLicense = flag.String("newrelic-license", "", "NewRelic license")
	flagDeviceDetector  = flag.String("device-detector-regexes", "regexes", "Folder with device detector regexes")
)

func main() {
	flag.Parse()
	newRelicApp, err := newrelic.NewApplication(
		newrelic.ConfigAppName("BTymQRbot"),
		newrelic.ConfigLicense(*flagNewRelicLicense),
		newrelic.ConfigAppLogForwardingEnabled(true),
	)
	if err != nil {
		log.Printf("Can't initialize NewRelic: %v", err)
	}
	imageService := groupme.NewImageService(*flagUserToken)
	messageService := groupme.NewMessageService(*flagBotToken)
	tymujClient := tymuj.NewClient(*flagTymujLogin, *flagTymujPassword, *flagTymujTeamID)
	dbClient := database.NewClient(*flagDbURL)
	ctx := context.Background()
	sheetOperator, err := google.NewSheetOperator(ctx, *flagGoogleSheetID)
	if err != nil {
		log.Printf("Can't initialize Google sheet client: %v", err)
	}
	driveOperator, err := google.NewDriveOperator(ctx)
	if err != nil {
		log.Printf("Can't initialize Google sheet client: %v", err)
	}
	csobClient := bank.NewCsobClient(*flagAccountNumber, dbClient)

	cronWorker := NewCronWorker(csobClient, sheetOperator, tymujClient, messageService, dbClient)
	locationPrague, err := time.LoadLocation("Europe/Prague")
	if err != nil {
		log.Printf("Error loading timezone:", err)
	}
	c := cron.NewWithLocation(locationPrague)
	c.AddFunc("0 */10 * * * *", func() { cronWorker.CheckNewPayments() })
	c.AddFunc("0 0 9 * * *", func() { cronWorker.CheckUnprocessedPayments() })
	c.AddFunc("0 0 12 * * 4", func() { cronWorker.CreateWednesdayEventForPlayers() })
	c.AddFunc("0 0 12 * * 4", func() { cronWorker.CreateWednesdayEventForGoalies() })
	c.Start()
	defer c.Stop()

	handler := NewHandler(newRelicApp, imageService, messageService, tymujClient, sheetOperator, driveOperator, *flagBotID, dbClient, csobClient, *flagDeviceDetector)
	fmt.Printf("Starting server...")
	err = http.ListenAndServe(*flagPort, handler.Mux())
	if errors.Is(err, http.ErrServerClosed) {
		log.Printf("server closed")
	} else if err != nil {
		log.Printf("error starting server: %v", err)
		os.Exit(1)
	} else {
		log.Printf("Server exited, err: %v", err)
	}
}
