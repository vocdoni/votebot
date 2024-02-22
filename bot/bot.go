package bot

import (
	"context"
	_ "embed"
	"sync"
	"time"

	"github.com/vocdoni/votebot/api"
	"go.vocdoni.io/dvote/log"
)

// defaultCoolDown is the default time to wait between casts
const defaultCoolDown = time.Second * 30

type BotConfig struct {
	API      api.API
	BotFID   uint64
	CoolDown time.Duration
}

type PollCallback func(*Poll) (string, error)

type Bot struct {
	api         api.API
	fid         uint64
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
	if config.API == nil {
		return nil, ErrAPINotSet
	}
	if config.BotFID == 0 {
		return nil, ErrBotFIDNotSet
	}
	if config.CoolDown == 0 {
		config.CoolDown = defaultCoolDown
	}
	return &Bot{
		api:         config.API,
		fid:         config.BotFID,
		waiter:      sync.WaitGroup{},
		polls:       make(chan *Poll),
		coolDown:    config.CoolDown,
		lastCast:    uint64(time.Now().Unix()),
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
				messages, lastCast, err := b.api.LastMentions(b.ctx, b.lastCast)
				if err != nil && err != ErrNoNewCasts {
					log.Errorf("error retrieving new casts: %s", err)
				}
				b.lastCast = lastCast
				if len(messages) > 0 {
					for _, msg := range messages {
						// parse message
						poll, err := ParsePoll(msg.Author, msg.Hash, msg.Content)
						if err != nil {
							log.Errorf("error parsing poll from message: %s", err)
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
				if cb := b.callback; cb != nil {
					url, err := (*cb)(poll)
					if err != nil {
						log.Errorf("error executing callback: %s", err)
						continue
					}
					if err := b.api.Reply(ctx, poll.Author, poll.MessageHash, url); err != nil {
						log.Errorf("error replying to poll: %s", err)
						continue
					}
					log.Infow("replied to poll", "poll", poll, "url", url)
				}
				b.callbackMtx.Unlock()
			}
		}
	}()
}

func (b *Bot) Stop() {
	if err := b.api.Stop(); err != nil {
		log.Errorf("error stopping bot: %s", err)
	}
	b.cancel()
	close(b.polls)
}
