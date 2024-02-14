package bot

import (
	"context"
	_ "embed"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"go.vocdoni.io/dvote/log"
)

const (
	farcasterEpoch int64 = 1609459200 // January 1, 2021 UTC
	lastCastFile         = "lastcast.txt"
	// defaultCoolDown is the default time to wait between casts
	defaultCoolDown = time.Second * 10
)

type BotConfig struct {
	BotFID     uint64
	Endpoint   string
	CoolDown   time.Duration
	PrivateKey string
}

type PollCallback func(*Poll) (string, error)

type Bot struct {
	fid         uint64
	privKey     []byte
	endpoint    string
	ctx         context.Context
	cancel      context.CancelFunc
	waiter      sync.WaitGroup
	polls       chan *Poll
	coolDown    time.Duration
	lastCast    uint64
	callback    *PollCallback
	callbackMtx sync.Mutex
}

func New(config BotConfig) (*Bot, error) {
	log.Infow("initializing bot", "config", config)
	if config.BotFID == 0 {
		return nil, ErrBotFIDNotSet
	}
	if config.PrivateKey == "" {
		return nil, ErrPrivateKeyNotSet
	}
	if config.Endpoint == "" {
		return nil, ErrEndpointNotSet
	}
	if config.CoolDown == 0 {
		config.CoolDown = defaultCoolDown
	}
	privKey, err := hex.DecodeString(config.PrivateKey)
	if err != nil {
		return nil, errors.Join(ErrDecodingPrivateKey, err)
	}
	lastCast := uint64(time.Now().Unix() - farcasterEpoch)
	lastCast = 0
	return &Bot{
		fid:         config.BotFID,
		privKey:     privKey,
		endpoint:    config.Endpoint,
		waiter:      sync.WaitGroup{},
		polls:       make(chan *Poll),
		coolDown:    config.CoolDown,
		lastCast:    lastCast,
		callback:    nil,
		callbackMtx: sync.Mutex{},
	}, nil
}

func (b *Bot) SetCallback(callback PollCallback) {
	b.callbackMtx.Lock()
	defer b.callbackMtx.Unlock()
	b.callback = &callback
}

func (b *Bot) Start(ctx context.Context) {
	b.ctx, b.cancel = context.WithCancel(ctx)

	b.waiter.Add(1)
	go func() {
		defer b.waiter.Done()

		ticker := time.NewTicker(b.coolDown)
		for {
			select {
			case <-b.ctx.Done():
				return
			default:
				log.Debugw("checking for new casts", "last-cast", b.lastCast)
				// retrieve new messages from the last cast
				messages, lastCast, err := b.GetLastsMentions(b.lastCast)
				if err != nil && err != ErrNoNewCasts {
					log.Errorf("error retrieving new casts: %s", err)
				}
				b.lastCast = lastCast
				if len(messages) > 0 {
					for _, message := range messages {
						// parse message
						poll, err := ParsePoll(message.ParentURL(), message.Mention())
						if err != nil {
							log.Errorf("error parsing poll from message '%s': %s", message.Mention(), err)
							continue
						}
						// send poll to the channel
						b.polls <- poll
					}
				} else {
					log.Debugw("no new casts", "last-cast", b.lastCast)
				}
				<-ticker.C
			}
		}
	}()

	b.waiter.Add(1)
	go func() {
		defer b.waiter.Done()
		for {
			select {
			case <-b.ctx.Done():
				return
			case poll := <-b.polls:
				b.callbackMtx.Lock()
				if b.callback != nil {
					res, err := (*b.callback)(poll)
					if err != nil {
						log.Errorf("error executing callback: %s", err)
						continue
					}
					log.Infow("callback executed, sending response", "response", res)
					b.Reply(poll.ParentURL, res)
				}
				b.callbackMtx.Unlock()
			}
		}
	}()
}

func (b *Bot) Stop() {
	b.cancel()
	close(b.polls)
}
