package main

import (
	"context"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/vocdoni/votebot/api"
	"github.com/vocdoni/votebot/api/hub"
	"github.com/vocdoni/votebot/api/neynar"
	"github.com/vocdoni/votebot/bot"
	"github.com/vocdoni/votebot/election"
	"github.com/vocdoni/votebot/poll"
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
	// onvote flags
	onvoteEndpoint := flag.String("onvoteEndpoint", "https://dev.farcaster.vote", "onvote frame generator http API endpoint")
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
	// check onvote endpoint
	if *onvoteEndpoint == "" {
		log.Fatal("onvote endpoint is required")
	}

	// set up the bot with the given configuration and the initialized API
	voteBot, err := bot.New(bot.BotConfig{
		CoolDown: *coolDown,
		API:      botAPI,
	})
	if err != nil {
		log.Fatal(err)
	}
	// start a context and a cancel function for the bot and start listening for
	// new casts
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case msg := <-voteBot.Messages:
				// when a new cast is received, check if it is a mention and if
				// it is not, continue to the next cast
				if !msg.IsMention {
					continue
				}
				// try to parse the message as a poll, if it fails continue to
				// the next cast
				poll, err := poll.ParseString(msg.Content, poll.DefaultConfig)
				if err != nil {
					log.Errorf("error parsing poll: %s", err)
					continue
				}
				// get the user data such as username, custody address and
				// verification addresses to create the election frame
				userdata, err := botAPI.UserData(ctx, msg.Author)
				if err != nil {
					log.Errorf("error getting user data: %s", err)
					continue
				}
				log.Infow("new poll",
					"poll", poll,
					"userdata", userdata)
				// create a new poll and send the result to the user
				frameURL, err := election.FrameElection(ctx, &election.ElectionOptions{
					BaseEndpoint: *onvoteEndpoint,
					Author: &election.Profile{
						FID:           msg.Author,
						Custody:       userdata.CustodyAddress,
						Verifications: userdata.VerificationsAddresses,
					},
					Question: poll.Question,
					Options:  poll.Options,
					Duration: int(poll.Duration.Hours()),
				})
				if err != nil {
					log.Errorf("error creating election frame: %s", err)
					continue
				}
				// compose the reply text and send it to the user as a reply to
				// the original cast
				replyText := fmt.Sprintf("Here is your election 🗳️ frame url! %s", frameURL)
				if err := botAPI.Reply(ctx, msg.Author, msg.Hash, replyText); err != nil {
					log.Errorf("error replying to cast: %s", err)
				}
			}
		}
	}()
	// start the bot
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
