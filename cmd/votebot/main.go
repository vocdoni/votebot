package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/vocdoni/votebot/bot"
	"go.vocdoni.io/dvote/log"
)

func main() {
	botFid := flag.Uint64("botFid", 0, "bot fid")
	endpoint := flag.String("endpoint", "https://hub.freefarcasterhub.com:3281", "API endpoint")
	authHeaders := flag.String("authHeaders", "", "auth headers")
	authKeys := flag.String("authKeys", "", "auth keys")
	privateKey := flag.String("privateKey", "0x0", "private key")
	coolDown := flag.Duration("cooldown", time.Second*30, "cooldown between casts")
	logLevel := flag.String("logLevel", "info", "log level")
	flag.Parse()
	// check required flags (bot fid and private key)
	if *botFid == 0 {
		log.Fatal("bot fid is required")
	}
	if *privateKey == "0x0" {
		log.Fatal("private key is required")
	}
	// check auth headers and keys, they must have the same length even if empty
	if (*authHeaders != "" && *authKeys == "") || (*authHeaders == "" && *authKeys != "") {
		log.Fatal("if authHeaders is set, authKeys must be set too and viceversa")
	}
	// create a map to store the auth headers and keys, parsing the given
	// strings separated by commas
	auth := make(map[string]string)
	headers := strings.Split(*authHeaders, ",")
	keys := strings.Split(*authKeys, ",")
	if len(headers) != len(keys) {
		log.Fatal("authHeaders and authKeys must have the same length")
	}
	for i, header := range headers {
		auth[header] = keys[i]
	}
	// init logger with the given log level
	log.Init(*logLevel, "stdout", nil)
	// set up the bot with the given configuration
	voteBot, err := bot.New(bot.BotConfig{
		BotFID:     *botFid,
		Endpoint:   *endpoint,
		Auth:       auth,
		CoolDown:   *coolDown,
		PrivateKey: *privateKey,
	})
	if err != nil {
		log.Fatal(err)
	}
	// set demo callback to return the URL of the vote app
	voteBot.SetCallback(func(poll *bot.Poll) (string, error) {
		log.Infow("poll received", "poll", poll)
		return "https://farcaster.vote/app", nil
	})
	// start the bot with a context and a cancel function
	ctx, cancel := context.WithCancel(context.Background())
	voteBot.Start(ctx)
	// wait for SIGTERM to cancel the context and stop the bot
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c
	log.Warnf("received SIGTERM, exiting at %s", time.Now().Format(time.RFC850))
	cancel()
	log.Info("waiting for routines to end gracefully...")
	// stop the bot and with a timeout of 5 seconds to give time to the
	// routines to end gracefully
	go func() {
		voteBot.Stop()
		log.Debug("all routines ended")
	}()
	time.Sleep(5 * time.Second)
	os.Exit(0)
}
