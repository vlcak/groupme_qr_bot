package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
)

var (
	flagBotToken      = flag.String("bot-token", "", "Bot TOKEN")
	flagBotID         = flag.String("bot-id", "", "Bot ID")
	flagPort          = flag.String("port", ":80", "Service address (e.g. :80)")
	flagUserToken     = flag.String("user-token", "", "User token for images")
	flagDbURL         = flag.String("db", "", "Database URL")
	flagTymujToken    = flag.String("tymuj-token", "", "Tymuj TOKEN")
	flagTymujTeamID   = flag.Int("tymuj-team-id", 33489, "Tymuj team ID")
	flagGoogleSheetID = flag.String("google-sheet-id", "", "Google sheet ID")
)

func main() {
	flag.Parse()
	imageService := NewImageService(*flagUserToken)
	messageService := NewMessageService(*flagBotToken)
	tymujClient := NewTymujClient(*flagTymujToken, *flagTymujTeamID)
	ctx := context.Background()
	sheetOperator, err := NewGoogleSheetOperator(ctx, *flagGoogleSheetID, "credentials.json")
	if err != nil {
		fmt.Printf("Can't initialize Google sheet client: %v\n", err)
	}
	handler := NewHandler(imageService, messageService, tymujClient, sheetOperator, *flagBotID, *flagDbURL)
	fmt.Printf("Starting server...\n")
	err = http.ListenAndServe(*flagPort, handler.Mux())
	if errors.Is(err, http.ErrServerClosed) {
		fmt.Printf("server closed\n")
	} else if err != nil {
		fmt.Printf("error starting server: %v\n", err)
		os.Exit(1)
	}
}
