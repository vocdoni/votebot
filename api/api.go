package api

import "context"

type API interface {
	Init(...any) error
	Stop() error
	LastMentions(ctx context.Context, timestamp uint64) ([]APIMessage, uint64, error)
	Reply(ctx context.Context, fid uint64, hash string, content string) error
	UserData(ctx context.Context, fid uint64) (string, string, []string, error)
}

type APIMessage struct {
	IsMention bool
	Content   string
	Author    uint64
	Hash      string
}
