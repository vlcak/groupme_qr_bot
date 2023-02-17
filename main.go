package main

import (
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
)

var (
	flagBotToken = flag.String("bot-token", "", "Bot TOKEN")
	flagBotID    = flag.String("bot-id", "", "Bot ID")
	flagPort     = flag.String("port", ":80", "Service address (e.g. :80)")
)

func main() {
	flag.Parse()
	messageSender := NewMessageSender(*flagBotToken)
	handler := NewHandler(messageSender, *flagBotID)
	fmt.Printf("Starting server...\n")
	err := http.ListenAndServe(*flagPort, handler.Mux())
	if errors.Is(err, http.ErrServerClosed) {
		fmt.Printf("server closed\n")
	} else if err != nil {
		fmt.Printf("error starting server: %s\n", err)
		os.Exit(1)
	}
}
