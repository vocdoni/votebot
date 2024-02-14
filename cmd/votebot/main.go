package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/vocdoni/votebot/bot"
	"go.vocdoni.io/dvote/log"
)

func main() {
	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = "info"
	}
	// init logger
	log.Init(logLevel, "stdout", nil)

	voteBot, err := bot.New(bot.BotConfig{
		BotFID:     0,
		Endpoint:   "",
		CoolDown:   time.Second * 30,
		PrivateKey: "",
	})
	if err != nil {
		log.Fatal(err)
	}

	voteBot.SetCallback(func(poll *bot.Poll) (string, error) {
		log.Infow("poll received", "poll", poll)
		return "poll received", nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	voteBot.Start(ctx)
	// wait for SIGTERM
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c
	log.Warnf("received SIGTERM, exiting at %s", time.Now().Format(time.RFC850))
	cancel()
	log.Info("waiting for routines to end gracefully...")
	// closing database
	go func() {
		voteBot.Stop()
		log.Debug("all routines ended")
	}()
	time.Sleep(5 * time.Second)
	os.Exit(0)
}
