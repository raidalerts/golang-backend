package raid

import (
	"context"
	"fmt"
	"strconv"

	firebase "firebase.google.com/go"
	"firebase.google.com/go/messaging"
	log "github.com/sirupsen/logrus"
)

type PushNotificator struct {
	topic       string
	firebaseApp *firebase.App
	fcmClient   *messaging.Client
	ctx context.Context
}

func NewPushNotificator(topic string) *PushNotificator {
	ctx := context.Background()
	app, err := firebase.NewApp(ctx, nil)
	if err != nil {
		log.Fatalf("error initializing app: %v\n", err)
		panic(err)
	}
	client, err := app.Messaging(ctx)
	if err != nil {
		log.Fatalf("error getting Messaging client: %v\n", err)
		panic(err)
	}
	return &PushNotificator{topic: topic, firebaseApp: app, fcmClient: client, ctx: ctx}
}

func (self *PushNotificator) Send(payload AiResponse) {
	message := &messaging.Message{
		Data: map[string]string {
			"alert": strconv.FormatBool(payload.Alert),
			"attacker": payload.Attacker,
			"text": payload.Text,
		},
		Notification: &messaging.Notification{
			Title: fmt.Sprintf("Alert: %s", payload.Attacker),
			Body: payload.Text,
		},
		Android: &messaging.AndroidConfig{
			Priority: "high",
		},
		Topic: self.topic,
	}

	// Send a message to the devices subscribed to the provided topic.
	response, err := self.fcmClient.Send(self.ctx, message)
	if err != nil {
		log.Error(err)
		return
	}
	log.Infof("Successfully sent message: %v", response)
}
