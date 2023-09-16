package raid

import (
	"os"
	"time"

	"github.com/caarlos0/env/v6"
	yaml "github.com/goccy/go-yaml"
	log "github.com/sirupsen/logrus"
)

type Settings struct {
	AlertChannel 		string         `env:"ALERT_CHANNEL" yaml:"alert_channel"`
	InfoChannel			string		   `env:"INFO_CHANNEL" yaml:"info_channel"`
	TimezoneName    	string         `env:"TZ" envDefault:"Europe/Kiev" yaml:"timezone_name"`
	Timezone        	*time.Location ``
	Debug           	bool           `env:"DEBUG" envDefault:"false" yaml:"debug"`
	Trace           	bool           `env:"TRACE" envDefault:"false" yaml:"trace"`
	AlertBacklogSize    int            `env:"ALERT_BACKLOG_SIZE" envDefault:"50" yaml:"alert_backlog_size"`
	InfoBacklogSize     int            `env:"INFO_BACKLOG_SIZE" envDefault:"5" yaml:"info_backlog_size"`
	RegionToMonitor		int			   `env:"REGION_TO_MONITOR" yaml:"region_to_monitor"`
	CityToMonitor		string		   `env:"CITY_TO_MONITOR" yaml:"city_to_monitor"`
	AnalyzerPrompt		string		   `yaml:"analyzer_prompt"`
	Topic				string		   `yaml:"push_topic"`
}

func MustLoadSettings() (settings Settings) {
	var err error

	settings.TimezoneName = "Europe/Kiev"
	settings.AlertChannel = "air_alert_ua"
	settings.InfoChannel = "vanek_nikolaev"

	if len(os.Args) > 1 {
		var f *os.File

		f, err = os.Open(os.Args[1])
		if err != nil {
			log.Fatalf("settings: open settings file: %s", err)
		}

		dec := yaml.NewDecoder(f)
		if err = dec.Decode(&settings); err != nil {
			log.Fatalf("settings: load settings from file: %s", err)
		}
	} else {
		opts := env.Options{
			RequiredIfNoDef: true,
			OnSet: func(tag string, value interface{}, isDefault bool) {
				if isDefault {
					log.Warnf("settings: using default value for env var %s: %s", tag, value)
				}
			},
		}
		if err = env.Parse(&settings, opts); err != nil {
			log.Fatalf("settings: load: %s", err)
		}
	}

	if settings.Timezone, err = time.LoadLocation(settings.TimezoneName); err != nil {
		log.Fatalf("settings: load timezone: %s", err)
	}

	log.Infof("settings: %v", settings)

	return
}
