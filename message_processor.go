package main

import (
	"encoding/json"
	"fmt"
	"io"
)

type GroupmeMessage struct {
	Attachments []interface{} `json:"attachments"`
	AvatarUrl   string        `json:"avatar_url"`
	CreatedAt   int64         `json:"created_at"`
	GroupId     string        `json:"group_id"`
	Id          string        `json:"id"`
	Name        string        `json:"name"`
	SenderId    string        `json:"sender_id"`
	SenderType  string        `json:"sender_type"`
	SourceGuid  string        `json:"source_guid"`
	System      bool          `json:"system"`
	Text        string        `json:"text"`
	UserId      string        `json:"user_id"`
}

func NewMessageProcessor(messageSender *MessageSender) *MessageProcessor {
	m := &MessageProcessor{
		messageSender: messageSender,
	}
	return m
}

type MessageProcessor struct {
	messageSender *MessageSender
}

func (mp *MessageProcessor) ProcessMessage(body io.ReadCloser) error {
	m := GroupmeMessage{}
	if err := json.NewDecoder(body).Decode(&m); err != nil {
		fmt.Printf("ERROR: %v\n", err)
		return err
	}
	fmt.Printf("Message text: %s\n", m.Text)
	mp.messageSender.SendMessage("Hello from BOT!", "")

	return nil
}
