package main

import (
	"github.com/gamebtc/devicedetector"
	"github.com/gamebtc/devicedetector/parser"
	"github.com/newrelic/go-agent/v3/newrelic"
	"github.com/vlcak/groupme_qr_bot/bank"
	"github.com/vlcak/groupme_qr_bot/db"
	"github.com/vlcak/groupme_qr_bot/google"
	"github.com/vlcak/groupme_qr_bot/groupme"
	"github.com/vlcak/groupme_qr_bot/tymuj"
	"io"
	"log"
	"net/http"
)

type Handler struct {
	handler           *http.ServeMux
	messageProcessor  *MessageProcessor
	deviceDetector    *devicedetector.DeviceDetector
	accountURL        string
	paymentsURL       string
	mobilePaymentsURL string
	tymujURL          string
}

// NewHandler creates a named service handler e.g. "conversations"
// Options may be supplied or set later with Option()
func NewHandler(
	newRelicApp *newrelic.Application,
	imageService *groupme.ImageService,
	messageService *groupme.MessageService,
	tymujClient *tymuj.Client,
	sheetOperator *google.SheetOperator,
	driveOperator *google.DriveOperator,
	botID string,
	dbClient *database.Client,
	bankClient *bank.CsobClient,
	deviceDetectorRegexes string,
) *Handler {
	h := &Handler{}
	h.messageProcessor = NewMessageProcessor(imageService, messageService, tymujClient, sheetOperator, driveOperator, botID, dbClient)
	h.accountURL = bankClient.GetAccountURL()
	h.paymentsURL = sheetOperator.GetReadOnlyURL()
	h.mobilePaymentsURL = sheetOperator.GetReadOnlyURLToSheet(1)
	h.tymujURL = tymuj.BaseURL
	h.handler = http.NewServeMux()
	h.handler.HandleFunc(newrelic.WrapHandleFunc(newRelicApp, "/", h.getRoot))
	h.handler.HandleFunc(newrelic.WrapHandleFunc(newRelicApp, "/message", h.messageReceived))
	h.handler.HandleFunc(newrelic.WrapHandleFunc(newRelicApp, "/platby", h.redirectToPaymetns))
	h.handler.HandleFunc(newrelic.WrapHandleFunc(newRelicApp, "/tymuj", h.messageReceived))
	h.handler.HandleFunc(newrelic.WrapHandleFunc(newRelicApp, "/ucet", h.messageReceived))
	var err error
	h.deviceDetector, err = devicedetector.NewDeviceDetector(deviceDetectorRegexes)
	if err != nil {
		log.Printf("Can't initialize device detector: %v, path: %s", err, deviceDetectorRegexes)
	}
	return h
}

func (h *Handler) getRoot(w http.ResponseWriter, r *http.Request) {
	log.Printf("Got ROOT request to %s", r.Host)
	var deviceInfo *devicedetector.DeviceInfo
	if h.deviceDetector != nil {
		deviceInfo = h.deviceDetector.Parse(r.UserAgent())
		log.Printf("Device type: %s, bot: %t", parser.GetDeviceName(deviceInfo.GetDeviceType()), deviceInfo.IsBot())
	}
	switch r.Host {
	case "platby.b-tym.cz":
		if deviceInfo != nil && deviceInfo.IsMobile() {
			http.Redirect(w, r, h.mobilePaymentsURL, http.StatusFound)
		} else {
			http.Redirect(w, r, h.paymentsURL, http.StatusFound)
		}
		return
	case "tymuj.b-tym.cz":
		http.Redirect(w, r, h.tymujURL, http.StatusFound)
		return
	case "ucet.b-tym.cz":
		http.Redirect(w, r, h.accountURL, http.StatusFound)
		return
	}
	io.WriteString(w, "Hello\n")
}

func (h *Handler) messageReceived(w http.ResponseWriter, r *http.Request) {
	log.Printf("Got MESSAGE request\n")
	h.messageProcessor.ProcessMessage(r.Body)
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) redirectToAccount(w http.ResponseWriter, r *http.Request) {
	log.Printf("Got ACCOUNT request\n")
	http.Redirect(w, r, h.accountURL, http.StatusFound)
}

func (h *Handler) redirectToPaymetns(w http.ResponseWriter, r *http.Request) {
	log.Printf("Got PAYMENTS request\n")
	http.Redirect(w, r, h.paymentsURL, http.StatusFound)
}

func (h *Handler) redirectToTymuj(w http.ResponseWriter, r *http.Request) {
	log.Printf("Got TYMUJ request\n")
	http.Redirect(w, r, h.tymujURL, http.StatusFound)
}

func (h *Handler) Mux() *http.ServeMux {
	return h.handler
}
