package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

const (
	BotURL = "https://api.groupme.com/v3/bots/post"
)

type ImageAttachment struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

type Message struct {
	BotId       string            `json:"bot_id"`
	Text        string            `json:"text"`
	Attachments []ImageAttachment `json:"attachments"`
}

func NewMessageSender(botToken string) *MessageSender {
	sender := &MessageSender{
		botId: botToken,
	}
	return senders
}

type MessageSender struct {
	botId string
}

func (ms *MessageSender) SendMessage(text, imageURL string) error {
	var attachments []ImageAttachment
	if imageURL != "" {
		attachments = append(attachments, ImageAttachment{
			Type: "image",
			URL:  imageURL,
		})
	}
	message := &Message{
		BotId:       ms.botId,
		Text:        text,
		Attachments: attachments,
	}

	body, err := json.Marshal(message)

	client := &http.Client{}
	response, err := client.Post(BotURL, "application/json", bytes.NewReader(body))
	if err != nil {
		fmt.Printf("Error sending the message: %v\n", err)
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		fmt.Printf("Unexpected return code: %d\n", response.StatusCode)
	}
	return nil
}
