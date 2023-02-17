package main

import (
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
)

var (
	flagBotID = flag.String("bot-id", "", "Bot ID")
)

func main() {
	flag.Parse()
	messageSender := NewMessageSender(*flagBotID)
	handler := NewHandler(messageSender)
	err := http.ListenAndServe(":8090", handler.Mux())
	if errors.Is(err, http.ErrServerClosed) {
		fmt.Printf("server closed\n")
	} else if err != nil {
		fmt.Printf("error starting server: %s\n", err)
		os.Exit(1)
	}
}
