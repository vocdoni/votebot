package main

import (
	"context"
	"encoding/hex"
	"flag"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/vocdoni/votebot/api"
	"github.com/vocdoni/votebot/api/hub"
	"github.com/vocdoni/votebot/api/neynar"
	"github.com/vocdoni/votebot/bot"
	"go.vocdoni.io/dvote/log"
)

func main() {
	botFid := flag.Uint64("botFid", 0, "bot fid")
	mode := flag.String("mode", "", "bot mode: neynar or hub")
	coolDown := flag.Duration("cooldown", time.Second*30, "cooldown between casts")
	logLevel := flag.String("logLevel", "info", "log level")
	// neynar mode flags
	neynarSignerUUID := flag.String("neynarSignerUUID", "", "neynar signer UUID")
	neynarAPIKey := flag.String("neynarAPIKey", "", "neynar API key")
	neynarEndpoint := flag.String("neynarEndpoint", "https://api.neynar.com", "neynar http API endpoint")
	// hub mode flags
	hubPrivateKey := flag.String("hubPrivateKey", "", "hub private key")
	hubEndpoint := flag.String("hubEndpoint", "https://hub.freefarcasterhub.com:3281", "hub http API endpoint")
	hubAuthHeaders := flag.String("hubAuthHeaders", "", "hub auth headers")
	hubAuthKeys := flag.String("hubAuthKeys", "", "hub auth keys")
	flag.Parse()
	// init logger with the given log level
	log.Init(*logLevel, "stdout", nil)
	// check required flags (bot fid and private key)
	if *botFid == 0 {
		log.Fatal("bot fid is required")
	}
	// check bot mode to initialize the API
	var botAPI api.API
	switch *mode {
	case "neynar":
		if *neynarSignerUUID == "" {
			log.Fatal("neynar signer UUID is required")
		}
		if *neynarAPIKey == "" {
			log.Fatal("neynar API key is required")
		}
		if *neynarEndpoint == "" {
			log.Fatal("neynar endpoint is required")
		}
		botAPI = new(neynar.NeynarAPI)
		if err := botAPI.Init(*botFid, *neynarSignerUUID, *neynarAPIKey, *neynarEndpoint); err != nil {
			log.Fatalf("error initializing neynar API: %s", err)
		}
	case "hub":
		if *hubPrivateKey == "" {
			log.Fatal("private key is required")
		}
		if *hubEndpoint == "" {
			log.Fatal("hub endpoint is required")
		}
		bHubPrivKey, err := hex.DecodeString(strings.TrimPrefix(*hubPrivateKey, "0x"))
		if err != nil {
			log.Fatalf("error decoding private key: %s", err)
		}
		// check auth headers and keys, they must have the same length even if empty
		if (*hubAuthHeaders != "" && *hubAuthKeys == "") || (*hubAuthHeaders == "" && *hubAuthKeys != "") {
			log.Fatal("if authHeaders is set, authKeys must be set too and viceversa")
		}
		// create a map to store the auth headers and keys, parsing the given
		// strings separated by commas
		hubAuth := make(map[string]string)
		headers := strings.Split(*hubAuthHeaders, ",")
		keys := strings.Split(*hubAuthKeys, ",")
		if len(headers) != len(keys) {
			log.Fatal("authHeaders and authKeys must have the same length")
		}
		for i, header := range headers {
			hubAuth[header] = keys[i]
		}
		botAPI = new(hub.Hub)
		if err := botAPI.Init(*botFid, bHubPrivKey, *hubEndpoint, hubAuth); err != nil {
			log.Fatalf("error initializing hub API: %s", err)
		}
	default:
		log.Fatal("'hub' or 'neynar' mode is required")
	}
	// set up the bot with the given configuration
	voteBot, err := bot.New(bot.BotConfig{
		BotFID:   *botFid,
		CoolDown: *coolDown,
		API:      botAPI,
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
