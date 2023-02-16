package main

import (
	"errors"
	"fmt"
	"net/http"
	"os"
)

func main() {
	handler := NewHandler()
	err := http.ListenAndServe(":80", handler.Mux())
	if errors.Is(err, http.ErrServerClosed) {
		fmt.Printf("server closed\n")
	} else if err != nil {
		fmt.Printf("error starting server: %s\n", err)
		os.Exit(1)
	}
}
