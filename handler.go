package main

import (
	"io"
	"log"
	"net/http"
)

type Handler struct {
	handler          *http.ServeMux
	messageProcessor *MessageProcessor
}

// NewHandler creates a named service handler e.g. "conversations"
// Options may be supplied or set later with Option()
func NewHandler(
	imageService *ImageService,
	messageService *MessageService,
	tymujClient *TymujClient,
	sheetOperator *GoogleSheetOperator,
	botID,
	dbURL string,
) *Handler {
	h := &Handler{}
	h.messageProcessor = NewMessageProcessor(imageService, messageService, tymujClient, sheetOperator, botID, dbURL)
	h.handler = http.NewServeMux()
	h.handler.HandleFunc("/", h.getRoot)
	h.handler.HandleFunc("/message", h.messageReceived)
	return h
}

func (h *Handler) getRoot(w http.ResponseWriter, r *http.Request) {
	log.Printf("Got ROOT request\n")
	io.WriteString(w, "Hello\n")
}

func (h *Handler) messageReceived(w http.ResponseWriter, r *http.Request) {
	log.Printf("Got MESSAGE request\n")
	h.messageProcessor.ProcessMessage(r.Body)
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) Mux() *http.ServeMux {
	return h.handler
}
