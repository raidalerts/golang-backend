package main

import (
	"context"
	"os"
	"os/signal"
	"raidalerts/raid"
	"sync"
	"syscall"
	_ "time/tzdata"

	log "github.com/sirupsen/logrus"
)

func main() {
	settings := raid.MustLoadSettings()
	if settings.Debug {
		log.SetLevel(log.DebugLevel)
	}

	if settings.Trace {
		log.SetLevel(log.TraceLevel)
	}

	ctx, cancel := context.WithCancel(context.Background())
	wg := &sync.WaitGroup{}
	errch := make(chan error, 32)

	notificator := raid.NewPushNotificator(settings.Topic)

	alertMonitorState := &raid.AlertMonitorState{}
	alertMonitor := raid.NewAlertMonitor(settings.AlertChannel, settings.Timezone, settings.AlertBacklogSize, 
		alertMonitorState, settings.RegionToMonitor)

	tgMonitorState := &raid.TgChannelMonitorState{}
	tgMonitor := raid.NewTgChannelMonitor(settings.InfoChannel, settings.Timezone, settings.InfoBacklogSize, tgMonitorState)

	aiMonitor := raid.NewAiAssistant(alertMonitorState, tgMonitor.Updates, settings.RegionToMonitor, 
		settings.CityToMonitor, settings.AnalyzerPrompt, notificator)
	
	webhooksHandler := raid.NewWebhooksHandler(settings.Webhooks, aiMonitor.Updates)

	go alertMonitor.Run(ctx, wg, errch)
	go aiMonitor.Run(ctx, wg, errch)
	go tgMonitor.Run(ctx, wg, errch)
	go webhooksHandler.Run(ctx, wg, errch)

	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	select {
	case sig := <-c:
		log.Warnf("main: receive %v", sig)
	case err := <-errch:
		log.Warnf("main: child crashed: %v", err)
	}
	cancel()
	log.Warnf("main: waiting for all children to terminate")
	wg.Wait()

	log.Warnf("main: finished")
}
