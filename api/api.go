package api

import "context"

type API interface {
	// Init initializes the API with the given arguments
	Init(...any) error
	// Stop stops the API
	Stop() error
	// LastMentions retrieves the last mentions from the given timestamp, it
	// returns the messages in a slice of APIMessage, the last timestamp and an
	// error if something goes wrong
	LastMentions(ctx context.Context, timestamp uint64) ([]*APIMessage, uint64, error)
	// Reply replies to a cast of the given fid with the given hash and content,
	// it returns an error if something goes wrong
	Reply(ctx context.Context, fid uint64, hash string, content string) error
	// UserDataByFID retrieves the Userdata of the user with the given fid, if
	// something goes wrong, it returns an error
	UserDataByFID(ctx context.Context, fid uint64) (*Userdata, error)
	// UserDataByVerificationAddress retrieves the Userdata of the user with the
	// given verification address, if something goes wrong, it returns an error
	UserDataByVerificationAddress(ctx context.Context, address string) (*Userdata, error)
}

type APIMessage struct {
	IsMention bool
	Content   string
	Author    uint64
	Hash      string
}

type Userdata struct {
	FID                    uint64
	Username               string
	CustodyAddress         string
	VerificationsAddresses []string
}
