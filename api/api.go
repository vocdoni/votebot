package api

import "context"

type API interface {
	Init(...any) error
	LastMentions(ctx context.Context, timestamp uint64) ([]APIMessage, uint64, error)
	Reply(ctx context.Context, fid uint64, hash []byte, content string) error
}

type APIMessage struct {
	IsMention bool
	Content   string
	Author    uint64
	Hash      []byte
}
