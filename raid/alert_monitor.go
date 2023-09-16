package raid

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

type AlertMonitor struct {
	telegramChannel string
	timezone        *time.Location
	backlogSize     int
	alertMonitorState    *AlertMonitorState
	Updates         *Topic[AlertUpdate]
	regionToMonitor	int
}

type AlertMonitorState struct {
	Regions        []Region   `json:"regions"`
	LastUpdate    time.Time `json:"last_update"`
	LastMessageID int64     `json:"last_message_id"`
}

type Region struct {
	ID      int        `json:"id"`
	Name    string     `json:"name"`
	NameEn  string     `json:"name_en"`
	Alert   bool       `json:"alert"`
	Changed *time.Time `json:"changed"`
}

type AlertUpdate struct {
	IsFresh bool
	IsLast  bool
	Region   Region
}

func NewAlertMonitor(telegramChannel string, timezone *time.Location, backlogSize int, alertMonitorState *AlertMonitorState, regionToMonitor int) *AlertMonitor {
	if len(alertMonitorState.Regions) == 0 {
		alertMonitorState.Regions = []Region{
			{1, "Вінницька область", "Vinnytsia oblast", false, nil},
			{2, "Волинська область", "Volyn oblast", false, nil},
			{3, "Дніпропетровська область", "Dnipropetrovsk oblast", false, nil},
			{4, "Донецька область", "Donetsk oblast", false, nil},
			{5, "Житомирська область", "Zhytomyr oblast", false, nil},
			{6, "Закарпатська область", "Zakarpattia oblast", false, nil},
			{7, "Запорізька область", "Zaporizhzhia oblast", false, nil},
			{8, "Івано-Франківська область", "Ivano-Frankivsk oblast", false, nil},
			{9, "Київська область", "Kyiv oblast", false, nil},
			{10, "Кіровоградська область", "Kirovohrad oblast", false, nil},
			{11, "Луганська область", "Luhansk oblast", false, nil},
			{12, "Львівська область", "Lviv oblast", false, nil},
			{13, "Миколаївська область", "Mykolaiv oblast", false, nil},
			{14, "Одеська область", "Odesa oblast", false, nil},
			{15, "Полтавська область", "Poltava oblast", false, nil},
			{16, "Рівненська область", "Rivne oblast", false, nil},
			{17, "Сумська область", "Sumy oblast", false, nil},
			{18, "Тернопільська область", "Ternopil oblast", false, nil},
			{19, "Харківська область", "Kharkiv oblast", false, nil},
			{20, "Херсонська область", "Kherson oblast", false, nil},
			{21, "Хмельницька область", "Khmelnytskyi oblast", false, nil},
			{22, "Черкаська область", "Cherkasy oblast", false, nil},
			{23, "Чернівецька область", "Chernivtsi oblast", false, nil},
			{24, "Чернігівська область", "Chernihiv oblast", false, nil},
			{25, "м. Київ", "Kyiv", false, nil},
		}
	}

	return &AlertMonitor{
		telegramChannel,
		timezone,
		backlogSize,
		alertMonitorState,
		NewTopic[AlertUpdate](),
		regionToMonitor,
	}
}

func (u *AlertMonitor) Run(ctx context.Context, wg *sync.WaitGroup, errch chan error) {
	defer log.Debug("AlertMonitor: exit")

	defer wg.Done()
	wg.Add(1)
	log.Infof("AlertMonitor: Start monitoring alerts from %s", u.telegramChannel)
	cc := NewChannelClient(u.telegramChannel)

	var wait <-chan time.Time

	if u.alertMonitorState.LastMessageID == 0 {
		log.Infof("AlertMonitor: no previous ID, will fetch backlog")

		messages, err := cc.FetchLast(ctx, u.backlogSize)
		if err != nil {
			errch <- fmt.Errorf("AlertMonitor: fetch initial batch: %w", err)

			return
		}

		log.Infof("AlertMonitor: fetch %d last messages", len(messages))
		u.alertMonitorState.LastMessageID = messages[len(messages)-1].ID

		u.ProcessMessages(ctx, messages, false)

		u.alertMonitorState.LastUpdate = time.Now().In(u.timezone)

		wait = time.After(2 * time.Second)
	} else {
		log.Infof("AlertMonitor: continue from ID %d", u.alertMonitorState.LastMessageID)

		wait = time.After(0)
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-wait:
		}

		messages, err := cc.FetchNewer(ctx, u.alertMonitorState.LastMessageID)
		if err != nil {
			log.Error(err)

			wait = time.After(10 * time.Second)

			continue
		}

		u.alertMonitorState.LastUpdate = time.Now().In(u.timezone)

		if len(messages) > 0 {
			log.Infof("AlertMonitor: fetch %d new messages", len(messages))
			u.alertMonitorState.LastMessageID = messages[len(messages)-1].ID
			u.ProcessMessages(ctx, messages, true)

			wait = time.After(0)
		} else {
			wait = time.After(2 * time.Second)
		}
	}
}

func (u *AlertMonitor) ProcessMessages(ctx context.Context, messages []Message, isFresh bool) {
	updates := []AlertUpdate{}

	for _, msg := range messages {
		var (
			on    bool
			region *Region
		)

		if len(msg.Text) < 2 {
			log.Debugf("AlertMonitor: not enough text in message: %v", msg.Text)

			continue
		}

		sentence := msg.Text[1]

		switch {
		case strings.Contains(sentence, "Повітряна тривога"):
			on = true
		case strings.Contains(sentence, "Відбій"):
			on = false
		default:
			log.Errorf("AlertMonitor: don't know how to parse \"%s\"", sentence)
		}

		for index, other := range u.alertMonitorState.Regions {
			if strings.Contains(sentence, other.Name) {
				region = &u.alertMonitorState.Regions[index]
			}
		}

		if region == nil {
			log.Debugf("AlertMonitor: no known states found in \"%s\"", sentence)
		} else {
			t := msg.Date.In(u.timezone)
			region.Changed = &t
			region.Alert = on
			if region.ID == u.regionToMonitor {
				log.Infof("AlertMonitor: new state: %s (id=%d) -> %v", region.Name, region.ID, on)
			} else {
				log.Debugf("AlertMonitor: new state: %s (id=%d) -> %v", region.Name, region.ID, on)
			}
			updates = append(updates, AlertUpdate{
				IsFresh: isFresh,
				Region:   *region,
			})
		}
	}

	for i, update := range updates {
		if i == len(updates)-1 {
			update.IsLast = true
		}

		u.Updates.Broadcast(update)
	}
}

func (s *AlertMonitorState) FindRegion(id int) *Region {
	for i, region := range s.Regions {
		if region.ID == id {
			return &s.Regions[i]
		}
	}

	return nil
}
