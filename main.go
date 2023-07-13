package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"github.com/newrelic/go-agent/v3/newrelic"
	"github.com/robfig/cron"
	"github.com/vlcak/groupme_qr_bot/bank"
	"github.com/vlcak/groupme_qr_bot/db"
	"github.com/vlcak/groupme_qr_bot/google"
	"github.com/vlcak/groupme_qr_bot/groupme"
	"github.com/vlcak/groupme_qr_bot/tymuj"
	"log"
	"net/http"
	"os"
)

var (
	flagBotToken        = flag.String("bot-token", "", "Bot TOKEN")
	flagBotID           = flag.String("bot-id", "", "Bot ID")
	flagPort            = flag.String("port", ":80", "Service address (e.g. :80)")
	flagUserToken       = flag.String("user-token", "", "User token for images")
	flagDbURL           = flag.String("db", "", "Database URL")
	flagTymujToken      = flag.String("tymuj-token", "", "Tymuj TOKEN")
	flagTymujTeamID     = flag.Int("tymuj-team-id", 33489, "Tymuj team ID")
	flagGoogleSheetID   = flag.String("google-sheet-id", "", "Google sheet ID")
	flagAccountNumber   = flag.Int("account-number", 311396620, "Account number")
	flagCsobURL         = flag.String("csob-url", "https://www.csob.cz/et-npw-lta-view/api/detail/transactionList", "CSOB transaction list URI")
	flagNewRelicLicense = flag.String("newrelic-license", "", "NewRelic license")
)

func main() {
	flag.Parse()
	newRelicApp, err := newrelic.NewApplication(
		newrelic.ConfigAppName("BTymQRbot"),
		newrelic.ConfigLicense(*flagNewRelicLicense),
		newrelic.ConfigAppLogForwardingEnabled(true),
	)
	imageService := groupme.NewImageService(*flagUserToken)
	messageService := groupme.NewMessageService(*flagBotToken)
	tymujClient := tymuj.NewClient(*flagTymujToken, *flagTymujTeamID)
	dbClient := database.NewClient(*flagDbURL)
	ctx := context.Background()
	sheetOperator, err := google.NewSheetOperator(ctx, *flagGoogleSheetID)
	driveOperator, err := google.NewDriveOperator(ctx)
	if err != nil {
		log.Printf("Can't initialize Google sheet client: %v", err)
	}
	csobClient := bank.NewCsobClient(*flagAccountNumber, *flagCsobURL, dbClient)

	cronWorker := NewCronWorker(csobClient, sheetOperator, messageService, dbClient)
	c := cron.New()
	c.AddFunc("0 */10 * * * *", func() { cronWorker.CheckNewPayments() })
	c.Start()
	defer c.Stop()

	handler := NewHandler(newRelicApp, imageService, messageService, tymujClient, sheetOperator, driveOperator, *flagBotID, dbClient, csobClient)
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
