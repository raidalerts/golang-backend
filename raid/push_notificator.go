package raid

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"cloud.google.com/go/firestore"
	firebase "firebase.google.com/go"
	"firebase.google.com/go/messaging"
	log "github.com/sirupsen/logrus"
	"google.golang.org/api/iterator"
)

type PushNotificator struct {
	topic       string
	firebaseApp *firebase.App
	fcmClient   *messaging.Client
	firestore   *firestore.Client
	ctx         context.Context
}

type FcmTokenRecord struct {
	Timestamp int64  `firestore:"timestamp"`
	Token     string `firestore:"token"`
	Uid       string `firestore:"uid"`
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
	firestore, err := app.Firestore(ctx)
	if err != nil {
		log.Fatalf("error getting Firestore client: %v\n", err)
		panic(err)
	}
	return &PushNotificator{topic: topic, firebaseApp: app, fcmClient: client, firestore: firestore, ctx: ctx}
}

func (self *PushNotificator) sendChunk(tokens []string, payload AiResponse) {
	message := &messaging.MulticastMessage{
		Data: map[string]string{
			"alert":    strconv.FormatBool(payload.Alert),
			"attacker": payload.Attacker,
			"text":     payload.Text,
			"confidence": fmt.Sprintf("%.2f", payload.Confidence),
			"original_text": payload.OriginalText,
		},
		Notification: &messaging.Notification{
			Title: fmt.Sprintf("Alert: %s", payload.Attacker),
			Body:  payload.Text,
		},
		Android: &messaging.AndroidConfig{
			Priority: "high",
		},
		Tokens: tokens,
	}

	// Send a message to the devices subscribed to the provided topic.
	response, err := self.fcmClient.SendMulticast(self.ctx, message)
	if err != nil {
		log.Error(err)
		return
	}
	log.Infof("Successfully sent message: %v", response)
}

func (self *PushNotificator) Send(payload AiResponse) {
	iter := self.firestore.Collection("fcm_tokens").Documents(self.ctx)
	var tokens []string
	defer iter.Stop()
	for {	
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Error(err)
			continue
		}
		var tokenRecord FcmTokenRecord
		err = doc.DataTo(&tokenRecord)
		if err != nil {
			log.Error(err)
			continue
		}
		recordTimestamp := time.UnixMilli(tokenRecord.Timestamp)
		log.Infof("Found token %s for user %s, timestamp %s", tokenRecord.Token, tokenRecord.Uid, recordTimestamp)
		if time.Since(recordTimestamp).Hours() > 24 * 30 * 2 {
			log.Infof("Removing outdated token %s for user %s", tokenRecord.Token, tokenRecord.Uid)
			doc.Ref.Delete(self.ctx)
			continue
		}
		tokens = append(tokens, tokenRecord.Token)
	}
	if len(tokens) == 0 {
		log.Info("No tokens found")
		return	
	}
	log.Infof("Sending notification to %d devices", len(tokens))
	chunkSize := 500
	for i := 0; i < len(tokens); i += chunkSize {
		end := i + chunkSize
		if end > len(tokens) {
			end = len(tokens)
		}
		self.sendChunk(tokens[i:end], payload)
	}
}
