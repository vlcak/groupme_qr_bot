package main

import (
	"github.com/newrelic/go-agent/v3/newrelic"
	"github.com/vlcak/groupme_qr_bot/db"
	"github.com/vlcak/groupme_qr_bot/google"
	"github.com/vlcak/groupme_qr_bot/groupme"
	"github.com/vlcak/groupme_qr_bot/tymuj"
	"io"
	"log"
	"net/http"
)

type Handler struct {
	handler          *http.ServeMux
	messageProcessor *MessageProcessor
	redirectURL      string
}

// NewHandler creates a named service handler e.g. "conversations"
// Options may be supplied or set later with Option()
func NewHandler(
	newRelicApp *newrelic.Application,
	imageService *groupme.ImageService,
	messageService *groupme.MessageService,
	tymujClient *tymuj.Client,
	sheetOperator *google.SheetOperator,
	botID string,
	dbClient *database.Client,
) *Handler {
	h := &Handler{}
	h.messageProcessor = NewMessageProcessor(imageService, messageService, tymujClient, sheetOperator, botID, dbClient)
	h.redirectURL = sheetOperator.GetReadOnlyURL()
	h.handler = http.NewServeMux()
	h.handler.HandleFunc(newrelic.WrapHandleFunc(newRelicApp, "/", h.getRoot))
	h.handler.HandleFunc(newrelic.WrapHandleFunc(newRelicApp, "/message", h.messageReceived))
	return h
}

func (h *Handler) getRoot(w http.ResponseWriter, r *http.Request) {
	log.Printf("Got ROOT request to %s", r.Host)
	if r.Host == "platby.b-tym.cz" {
		http.Redirect(w, r, h.redirectURL, http.StatusFound)
		return
	}
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
