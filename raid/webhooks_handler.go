package raid

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"sync"

	log "github.com/sirupsen/logrus"
)

type WebhooksHandler struct {
	webhooks []WebhookConfig
	updates  *Topic[AiResponse]
}

func NewWebhooksHandler(webhooks []WebhookConfig, aiMessages *Topic[AiResponse]) *WebhooksHandler {
	return &WebhooksHandler{
		webhooks: webhooks,
		updates:  aiMessages,
	}
}

func (self *WebhooksHandler) Run(ctx context.Context, wg *sync.WaitGroup, errch chan error) {
	defer log.Debug("WebhooksHandler: exit")

	defer wg.Done()
	wg.Add(1)
	log.Infof("WebhooksHandler: configured %d webhooks", len(self.webhooks))
	log.Info("WebhooksHandler: waiting for updates")
	events := self.updates.Subscribe("WebhooksHandler", func(u AiResponse) bool {
		return true
	})

	defer func() {
		self.updates.Unsubscribe(events)
	}()

	for {

		select {
		case <-ctx.Done():
			return
		case event, ok := <-events:
			if !ok {
				return
			}
			for _, webhook := range self.webhooks {
				self.CallWebhook(ctx, webhook, event)
			}
		}
	}
}

func (self *WebhooksHandler) CallWebhook(ctx context.Context, webhook WebhookConfig, event AiResponse) {
	log.Infof("WebhooksHandler: calling webhook %s", webhook.Name)
	req, err := http.NewRequestWithContext(ctx, webhook.Method, webhook.Url, nil)
	if err != nil {
		log.Error(err)
		return
	}
	q := req.URL.Query()
	for _, queryArg := range webhook.QueryArgs {
		q.Add(queryArg.Name, queryArg.Value)
	}
	if webhook.Method == "GET" {
		for key, value := range webhook.Payload {
			switch value.(type) {
			case string:
				tmpl, err := template.New(key).Parse(value.(string))
				if err != nil {
					log.Error(err)
					return
				}
				var buf bytes.Buffer
				err = tmpl.Execute(&buf, event)
				if err != nil {
					log.Error(err)
					return
				}
				q.Add(key, buf.String())
			default:
				q.Add(key, fmt.Sprintf("%v", value))
			}
		}
	} else {
		var body map[string]interface{}
		for key, value := range webhook.Payload {
			switch value.(type) {
			case string:
				tmpl, err := template.New(key).Parse(value.(string))
				if err != nil {
					log.Error(err)
					return
				}
				var buf bytes.Buffer
				err = tmpl.Execute(&buf, event)
				if err != nil {
					log.Error(err)
					return
				}
				body[key] = buf.String()
			default:
				body[key] = value
			}
		}
		req.Header.Set("Content-Type", "application/json")
		payloadBuf := new(bytes.Buffer)
		json.NewEncoder(payloadBuf).Encode(body)
		req.Body = io.NopCloser(payloadBuf)
	}
	req.URL.RawQuery = q.Encode()
	client := &http.Client{}
	log.Debugf("WebhooksHandler: %s %s, body: %v", webhook.Method, req.URL.String(), req.Body)
	res, e := client.Do(req)
	if e != nil {
		log.Error(err)
		return
	}
	defer res.Body.Close()
	log.Infof("WebhooksHandler: webhook %s returned %v", webhook.Name, res.Status)
}
