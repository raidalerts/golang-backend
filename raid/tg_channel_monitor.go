package raid

import (
	"context"
	"fmt"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

type TgChannelMonitor struct {
	telegramChannel string
	timezone        *time.Location
	backlogSize     int
	monitorState    *TgChannelMonitorState
	Updates         *Topic[TgMessageUpdate]
}

type TgChannelMonitorState struct {
	LastUpdate    time.Time `json:"last_update"`
	LastMessageID int64     `json:"last_message_id"`
}

type TgMessageUpdate struct {
	IsFresh bool
	IsLast  bool
	Msg   Message
}

func NewTgChannelMonitor(telegramChannel string, timezone *time.Location, backlogSize int, monitorState *TgChannelMonitorState) *TgChannelMonitor {
	return &TgChannelMonitor{
		telegramChannel,
		timezone,
		backlogSize,
		monitorState,
		NewTopic[TgMessageUpdate](),
	}
}

func (u *TgChannelMonitor) Run(ctx context.Context, wg *sync.WaitGroup, errch chan error) {
	defer log.Debug("TgChannelMonitor: exit")

	defer wg.Done()
	wg.Add(1)

	log.Infof("TgChannelMonitor: Start monitoring messages from %s", u.telegramChannel)
	cc := NewChannelClient(u.telegramChannel)

	var wait <-chan time.Time

	if u.monitorState.LastMessageID == 0 {
		log.Infof("TgChannelMonitor: no previous ID, will fetch backlog")

		messages, err := cc.FetchLast(ctx, u.backlogSize)
		if err != nil {
			errch <- fmt.Errorf("TgChannelMonitor: fetch initial batch: %w", err)

			return
		}

		log.Infof("TgChannelMonitor: fetch %d last messages", len(messages))
		u.monitorState.LastMessageID = messages[len(messages)-1].ID

		u.ProcessMessages(ctx, messages, false)

		u.monitorState.LastUpdate = time.Now().In(u.timezone)

		wait = time.After(2 * time.Second)
	} else {
		log.Infof("TgChannelMonitor: continue from ID %d", u.monitorState.LastMessageID)

		wait = time.After(0)
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-wait:
		}

		messages, err := cc.FetchNewer(ctx, u.monitorState.LastMessageID)
		if err != nil {
			log.Error(err)

			wait = time.After(10 * time.Second)

			continue
		}

		u.monitorState.LastUpdate = time.Now().In(u.timezone)

		if len(messages) > 0 {
			log.Infof("TgChannelMonitor: fetch %d new messages", len(messages))
			u.monitorState.LastMessageID = messages[len(messages)-1].ID
			u.ProcessMessages(ctx, messages, true)

			wait = time.After(0)
		} else {
			wait = time.After(2 * time.Second)
		}
	}
}

func (u *TgChannelMonitor) ProcessMessages(ctx context.Context, messages []Message, isFresh bool) {
	updates := []TgMessageUpdate{}

	for _, msg := range messages {
		updates = append(updates, TgMessageUpdate{
			IsFresh: isFresh,
			Msg: msg,
		})
	}

	for i, update := range updates {
		if i == len(updates)-1 {
			update.IsLast = true
		}
		log.Debugf("TgChannelMonitor: broadcasting: \n %v", update)
		u.Updates.Broadcast(update)
	}
}
