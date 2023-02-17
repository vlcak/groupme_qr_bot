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

type Message struct {
	BotId string `json:"bot_id"`
	Text  string `json:"text"`
}

func NewMessageSender(botToken string) *MessageSender {
	sender := &MessageSender{
		botId: botToken,
	}
	return sender
}

type MessageSender struct {
	botId string
}

func (ms *MessageSender) SendMessage(text, image string) error {
	message := &Message{
		BotId: ms.botId,
		Text:  text,
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
