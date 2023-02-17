package main

import (
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
)

var (
	flagBotToken  = flag.String("bot-token", "", "Bot TOKEN")
	flagBotID     = flag.String("bot-id", "", "Bot ID")
	flagPort      = flag.String("port", ":80", "Service address (e.g. :80)")
	flagUserToken = flag.String("user-token", "", "User token for images")
	flagDbURL     = flag.String("db", "", "Database URL")
)

func main() {
	flag.Parse()
	imageService := NewImageService(*flagUserToken)
	messageService := NewMessageService(*flagBotToken)
	handler := NewHandler(imageService, messageService, *flagBotID, *flagDbURL)
	fmt.Printf("Starting server...\n")
	err := http.ListenAndServe(*flagPort, handler.Mux())
	if errors.Is(err, http.ErrServerClosed) {
		fmt.Printf("server closed\n")
	} else if err != nil {
		fmt.Printf("error starting server: %s\n", err)
		os.Exit(1)
	}
}
