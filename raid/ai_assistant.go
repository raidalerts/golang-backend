package raid

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/sashabaranov/go-openai"
	log "github.com/sirupsen/logrus"
)

type AiAssistant struct {
	alertMonitorState *AlertMonitorState
	infoUpdates       *Topic[TgMessageUpdate]
	regionToMonitor   int
	aiClient          openai.Client
	cityToMonitor     string
	prompt            string
	Updates           *Topic[AiResponse]
	notificator       *PushNotificator
}

type AiResponse struct {
	Alert        bool    `json:"alert"`
	Attacker     string  `json:"attacker"`
	Text         string  `json:"trigger"`
	Confidence   float32 `json:"confidence"`
	OriginalText string  `json:"original_text"`
}

func delete_empty(s []string) []string {
	var r []string
	for _, str := range s {
		if str != "" {
			r = append(r, str)
		}
	}
	return r
}

func NewAiAssistant(alertMonitorState *AlertMonitorState, infoUpdates *Topic[TgMessageUpdate],
	regionToMonitor int, cityToMonitor string, prompt string, notificator *PushNotificator) *AiAssistant {
	aiClient := openai.NewClient(os.Getenv("OPENAI_API_KEY"))
	return &AiAssistant{alertMonitorState, infoUpdates, regionToMonitor, *aiClient, cityToMonitor, prompt, NewTopic[AiResponse](), notificator}
}

func (self *AiAssistant) Run(ctx context.Context, wg *sync.WaitGroup, errch chan error) {
	defer log.Debug("AiAssistant: exit")

	defer wg.Done()
	wg.Add(1)
	log.Info("AiAssistant: waiting for updates")
	events := self.infoUpdates.Subscribe("AiAssistant", func(u TgMessageUpdate) bool {
		return u.IsFresh || u.IsLast
	})

	defer func() {
		self.infoUpdates.Unsubscribe(events)
	}()

	var wait <-chan time.Time
	wait = time.After(0)

	for {
		select {
		case <-ctx.Done():
			return
		case <-wait:
		}

		select {
		case <-ctx.Done():
			return
		case event, ok := <-events:
			wait = time.After(0)
			if !ok {
				return
			}
			msgText := strings.Join(delete_empty(event.Msg.Text), "\n")
			if self.alertMonitorState.Regions[self.regionToMonitor-1].Alert {
				wait = time.After(5 * time.Second)
				log.Infof("AiAssistant: received tg message:\n%s\n", msgText)
				req := openai.ChatCompletionRequest{
					Model: openai.GPT3Dot5Turbo0613,
					Messages: []openai.ChatCompletionMessage{
						{
							Role:    openai.ChatMessageRoleSystem,
							Content: fmt.Sprintf(self.prompt, self.cityToMonitor, self.cityToMonitor),
						},
					},
				}
				req.Messages = append(req.Messages, openai.ChatCompletionMessage{
					Role:    openai.ChatMessageRoleUser,
					Content: msgText,
				})
				resp, err := self.aiClient.CreateChatCompletion(context.Background(), req)
				if err != nil {
					errch <- fmt.Errorf("AiAssistant: ChatCompletion error: %v", err)
					continue
				}

				aiResponse := &AiResponse{Text: msgText, OriginalText: msgText}
				if err := json.Unmarshal([]byte(resp.Choices[0].Message.Content), &aiResponse); err != nil {
					errch <- fmt.Errorf("AiAssistant: failed to parse AI output: %v", err)
					continue
				}
				if aiResponse.Alert {
					log.Infof("AiAssistant: AI reports alert %v", aiResponse)
					self.Updates.Broadcast(*aiResponse)
					self.notificator.Send(*aiResponse)
				}
			} else {
				log.Debugf("AiAssistant: ignoring message:\n %s", msgText)
			}
		}
	}
}
