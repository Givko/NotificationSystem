package models

import "encoding/json"

type Notification struct {
	Recipient   string `json:"recipient"`
	RecipientID string `json:"recipient_id"`
	Sender      string `json:"sender"`
	SenderID    string `json:"sender_id"`
	Subject     string `json:"subject"`
	Message     string `json:"message"`
	Channel     string `json:"channel"`
}

func (n *Notification) ToJSON() ([]byte, error) {
	return json.Marshal(n)
}
