package bot

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/vocdoni/votebot/protobufs"
	"github.com/zeebo/blake3"
	"go.vocdoni.io/dvote/log"
	"google.golang.org/protobuf/proto"
)

const (
	ENDPOINT_CAST_BY_MENTION = "castsByMention?fid=%d"
	ENDPOINT_SUBMIT_MESSAGE  = "submitMessage"
	// timeouts
	getCastByMentionTimeout = 1 * time.Minute
)

type CastByMentionsResponse struct {
	Messages []*Message `json:"messages"`
}

func (b *Bot) GetLastsMentions(timestamp uint64) ([]*Message, uint64, error) {
	internalCtx, cancel := context.WithTimeout(b.ctx, getCastByMentionTimeout)
	defer cancel()
	// download de json from API endpoint
	baseEndpoint := fmt.Sprintf("%s/%s", b.endpoint, ENDPOINT_CAST_BY_MENTION)
	endpoint := fmt.Sprintf(baseEndpoint, b.fid)
	req, err := http.NewRequestWithContext(internalCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("error creating request: %w", err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("error downloading json: %w", err)
	}
	defer func() {
		if err := res.Body.Close(); err != nil {
			log.Error("error closing response body")
		}
	}()
	if res.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("error downloading json: %s", res.Status)
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, 0, fmt.Errorf("error reading response body: %w", err)
	}
	// unmarshal the json
	mentions := &CastByMentionsResponse{}
	if err := json.Unmarshal(body, mentions); err != nil {
		return nil, 0, fmt.Errorf("error unmarshalling json: %w", err)
	}
	// filter messages and calculate the last timestamp
	lastTimestamp := uint64(0)
	filteredMessages := []*Message{}
	for _, message := range mentions.Messages {
		if !message.IsMention() {
			continue
		}
		if message.Data.Timestamp > timestamp {
			if message.Data.Timestamp > lastTimestamp {
				lastTimestamp = message.Data.Timestamp
			}
			filteredMessages = append(filteredMessages, message)
		}
	}
	// if there are no new casts, return an error
	if len(filteredMessages) == 0 {
		return nil, timestamp, ErrNoNewCasts
	}
	// return the filtered messages and the last timestamp
	return filteredMessages, lastTimestamp, nil
}

func (b *Bot) Reply(targetFid uint64, targetHash []byte, text string) {
	// create the cast as a reply to the message with the parentFID provided
	// and the desired text
	castAdd := &protobufs.CastAddBody{
		Text: text,
		Parent: &protobufs.CastAddBody_ParentCastId{
			ParentCastId: &protobufs.CastId{
				Fid:  targetFid,
				Hash: targetHash,
			},
		},
	}
	// compose the message data with the message type, the bot FID, the current
	// timestamp, the network, and the cast add body
	msgData := &protobufs.MessageData{
		Type:      protobufs.MessageType_MESSAGE_TYPE_CAST_ADD,
		Fid:       b.fid,
		Timestamp: uint32(time.Now().Unix() - farcasterEpoch),
		Network:   protobufs.FarcasterNetwork_FARCASTER_NETWORK_MAINNET,
		Body:      &protobufs.MessageData_CastAddBody{castAdd},
	}
	// marshal the message data
	msgDataBytes, err := proto.Marshal(msgData)
	if err != nil {
		log.Errorf("error marshalling message data: %s", err)
	}
	// calculate the hash of the message data
	hasher := blake3.New()
	hasher.Write(msgDataBytes)
	hash := hasher.Sum(nil)[:20]
	// create the message with the hash scheme, the hash and the signature
	// scheme
	msg := &protobufs.Message{
		HashScheme:      protobufs.HashScheme_HASH_SCHEME_BLAKE3,
		Hash:            hash,
		SignatureScheme: protobufs.SignatureScheme_SIGNATURE_SCHEME_ED25519,
	}
	// sign the message with the private key
	privateKey := ed25519.NewKeyFromSeed(b.privKey)
	signature := ed25519.Sign(privateKey, msgDataBytes)
	signer := privateKey.Public().(ed25519.PublicKey)
	// set the signature and the signer to the message
	msg.Signature = signature
	msg.Signer = signer
	// set the message data bytes to the message and marshal the message
	msg.DataBytes = msgDataBytes
	msgBytes, err := proto.Marshal(msg)
	if err != nil {
		log.Fatalf("error marshalling message: %s", err)
	}
	// create a new context with a timeout
	internalCtx, cancel := context.WithTimeout(b.ctx, getCastByMentionTimeout)
	defer cancel()
	// submit the message to the API endpoint
	endpoint := fmt.Sprintf("%s/%s", b.endpoint, ENDPOINT_SUBMIT_MESSAGE)
	req, err := http.NewRequestWithContext(internalCtx, http.MethodPost, endpoint, bytes.NewBuffer(msgBytes))
	if err != nil {
		log.Fatalf("error creating request: %s", err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Errorf("error submitting the message: %s", err)
	}
	defer func() {
		if err := res.Body.Close(); err != nil {
			log.Error("error closing response body")
		}
	}()
	if res.StatusCode != http.StatusOK {
		log.Errorf("error submitting the message: %s", res.Status)
	}
}
