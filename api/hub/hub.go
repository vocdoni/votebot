package hub

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/vocdoni/votebot/api"
	"github.com/vocdoni/votebot/api/hub/protobufs"
	"github.com/zeebo/blake3"
	"go.vocdoni.io/dvote/log"
	"google.golang.org/protobuf/proto"
)

const (
	// endpoints
	ENDPOINT_CAST_BY_MENTION       = "castsByMention?fid=%d"
	ENDPOINT_SUBMIT_MESSAGE        = "submitMessage"
	ENDPOINT_USERNAME_PROOFS       = "userNameProofsByFid?fid=%d"
	ENDPOINT_VERIFICATIONS         = "verificationsByFid?fid=%d"
	ENDPOINT_IDREGISTRY_BY_ADDRESS = "onChainIdRegistryEventByAddress?address=%s"
	// timeouts
	getCastByMentionTimeout = 15 * time.Second
	submitMessageTimeout    = 5 * time.Minute
	userdataTimeout         = 15 * time.Second
	// message types
	MESSAGE_TYPE_CAST_ADD     = "MESSAGE_TYPE_CAST_ADD"
	MESSAGE_TYPE_USERPROOF    = "USERNAME_TYPE_FNAME"
	MESSAGE_TYPE_VERIFICATION = "MESSAGE_TYPE_VERIFICATION_ADD_ETH_ADDRESS"
	// other constants
	farcasterEpoch uint64 = 1609459200 // January 1, 2021 UTC
)

type Hub struct {
	fid      uint64
	privKey  []byte
	endpoint string
	auth     map[string]string
}

func (h *Hub) Init(args ...any) error {
	// parse arguments:
	// - botFID uint64
	// - privateKey []byte
	// - endpoint string
	// - auth map[string]string (optional)
	if len(args) < 3 {
		return fmt.Errorf("invalid number of arguments")
	}
	var ok bool
	if h.fid, ok = args[0].(uint64); !ok || h.fid == 0 {
		return fmt.Errorf("invalid type for botFID")
	}
	h.privKey, ok = args[1].([]byte)
	if !ok || len(h.privKey) == 0 {
		return fmt.Errorf("invalid type for privateKey")
	}
	if h.endpoint, ok = args[2].(string); !ok || h.endpoint == "" {
		return fmt.Errorf("invalid type for endpoint")
	}
	if len(args) > 3 {
		if auth, ok := args[3].(map[string]string); ok && len(auth) > 0 {
			h.auth = auth
		}
	}
	return nil
}

func (h *Hub) Stop() error {
	return nil
}

func (h *Hub) LastMentions(ctx context.Context, timestamp uint64) ([]*api.APIMessage, uint64, error) {
	if timestamp > farcasterEpoch {
		timestamp -= farcasterEpoch
	}
	internalCtx, cancel := context.WithTimeout(ctx, getCastByMentionTimeout)
	defer cancel()
	// download de json from API endpoint
	uri := fmt.Sprintf(ENDPOINT_CAST_BY_MENTION, h.fid)
	req, err := h.newRequest(internalCtx, http.MethodGet, uri, nil)
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
	mentions := &HubMentionsResponse{}
	if err := json.Unmarshal(body, mentions); err != nil {
		return nil, 0, fmt.Errorf("error unmarshalling json: %w", err)
	}
	// filter messages and calculate the last timestamp
	lastTimestamp := uint64(0)
	messages := []*api.APIMessage{}
	for _, m := range mentions.Messages {
		isMention := m.Data.Type == MESSAGE_TYPE_CAST_ADD && m.Data.CastAddBody != nil && m.Data.CastAddBody.Text != ""
		if !isMention {
			continue
		}
		if m.Data.Timestamp > timestamp {
			messages = append(messages, &api.APIMessage{
				IsMention: true,
				Content:   m.Data.CastAddBody.Text,
				Author:    m.Data.From,
				Hash:      m.HexHash,
			})
			if m.Data.Timestamp > lastTimestamp {
				lastTimestamp = m.Data.Timestamp
			}
		}
	}
	// if there are no new casts, return an error
	if len(messages) == 0 {
		return nil, timestamp, fmt.Errorf("no new casts")
	}
	// return the filtered messages and the last timestamp
	return messages, lastTimestamp + farcasterEpoch, nil
}

func (h *Hub) Reply(ctx context.Context, targetFid uint64, targetHash string, content string) error {
	// create the cast as a reply to the message with the parentFID provided
	// and the desired text
	bTargetHash, err := hex.DecodeString(strings.TrimPrefix(targetHash, "0x"))
	if err != nil {
		return fmt.Errorf("error decoding target hash: %s", err)
	}
	castAdd := &protobufs.CastAddBody{
		Text: content,
		// Mentions:          []uint64{targetFid},
		// MentionsPositions: []uint32{0},
		Parent: &protobufs.CastAddBody_ParentCastId{
			ParentCastId: &protobufs.CastId{
				Fid:  targetFid,
				Hash: bTargetHash,
			},
		},
	}
	// compose the message data with the message type, the bot FID, the current
	// timestamp, the network, and the cast add body
	msgData := &protobufs.MessageData{
		Type:      protobufs.MessageType_MESSAGE_TYPE_CAST_ADD,
		Fid:       h.fid,
		Timestamp: uint32(uint64(time.Now().Unix()) - farcasterEpoch),
		Network:   protobufs.FarcasterNetwork_FARCASTER_NETWORK_MAINNET,
		Body:      &protobufs.MessageData_CastAddBody{CastAddBody: castAdd},
	}
	// marshal the message data
	msgDataBytes, err := proto.Marshal(msgData)
	if err != nil {
		return fmt.Errorf("error marshalling message data: %s", err)
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
		Data:            msgData,
		DataBytes:       msgDataBytes,
	}
	// sign the message with the private key
	privateKey := ed25519.NewKeyFromSeed(h.privKey)
	signature := ed25519.Sign(privateKey, hash)
	signer := privateKey.Public().(ed25519.PublicKey)
	// set the signature and the signer to the message
	msg.Signature = signature
	msg.Signer = signer
	// marshal the message
	msgBytes, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("error marshalling message: %s", err)
	}
	// create a new context with a timeout
	internalCtx, cancel := context.WithTimeout(ctx, submitMessageTimeout)
	defer cancel()
	// submit the message to the API endpoint
	req, err := h.newRequest(internalCtx, http.MethodPost, ENDPOINT_SUBMIT_MESSAGE, bytes.NewBuffer(msgBytes))
	if err != nil {
		return fmt.Errorf("error creating request: %s", err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("error submitting the message: %s", err)
	}
	if res.StatusCode != http.StatusOK {
		// read the response body
		body, err := io.ReadAll(res.Body)
		if err != nil {
			return fmt.Errorf("error reading response body: %s", err)
		}
		return fmt.Errorf("error submitting the message: %s", string(body))
	}
	return nil
}

func (h *Hub) UserDataByFID(ctx context.Context, fid uint64) (*api.Userdata, error) {
	// create a intenal context with a timeout
	internalCtx, cancel := context.WithTimeout(ctx, userdataTimeout)
	defer cancel()
	// prepare the request to get username and custody address from the API
	usernameReq, err := h.newRequest(internalCtx, http.MethodGet, fmt.Sprintf(ENDPOINT_USERNAME_PROOFS, fid), nil)
	if err != nil {
		return nil, fmt.Errorf("error creating user data request: %w", err)
	}
	// download the user data from the API and check for errors
	usernameRes, err := http.DefaultClient.Do(usernameReq)
	if err != nil {
		return nil, fmt.Errorf("error downloading user data: %w", err)
	}
	if usernameRes.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("error downloading user data: %s", usernameRes.Status)
	}
	// read the response body
	usernameBody, err := io.ReadAll(usernameRes.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading user data response body: %w", err)
	}
	// unmarshal the json
	userdata := &UserdataResponse{}
	if err := json.Unmarshal(usernameBody, userdata); err != nil {
		return nil, fmt.Errorf("error unmarshalling user data: %w", err)
	}
	// get the latest proof
	currentUserdata := &UsernameProofs{}
	lastUserdataTimestamp := uint64(0)
	for _, proof := range userdata.Proofs {
		// discard proofs that are not of the type we are looking for and
		// that are not from the user we are looking for
		if proof.Type != MESSAGE_TYPE_USERPROOF || proof.FID != fid {
			continue
		}
		// update the latest proof
		if proof.Timestamp > lastUserdataTimestamp {
			currentUserdata = proof
			lastUserdataTimestamp = proof.Timestamp
		}
	}
	// prepare the request to get verifications from the API
	verificationsReq, err := h.newRequest(internalCtx, http.MethodGet, fmt.Sprintf(ENDPOINT_VERIFICATIONS, fid), nil)
	if err != nil {
		return nil, fmt.Errorf("error creating verifications request: %w", err)
	}
	// download the verifications from the API and check for errors
	verificationsRes, err := http.DefaultClient.Do(verificationsReq)
	if err != nil {
		return nil, fmt.Errorf("error downloading verifications: %w", err)
	}
	if verificationsRes.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("error downloading verifications: %s", verificationsRes.Status)
	}
	// read the response body
	verificationsBody, err := io.ReadAll(verificationsRes.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading verifications response body: %w", err)
	}
	// decode verifications json
	verificationsData := &VerificationsResponse{}
	if err := json.Unmarshal(verificationsBody, verificationsData); err != nil {
		return nil, fmt.Errorf("error unmarshalling verifications: %w", err)
	}
	// filter verifications addresses
	verifications := []string{}
	for _, msg := range verificationsData.Messages {
		// if no data or verification data is found, skip. If the message data
		// type is not the one we are looking for, skip
		if msg.Data == nil || msg.Data.Type != MESSAGE_TYPE_VERIFICATION || msg.Data.Verification == nil {
			continue
		}
		verifications = append(verifications, msg.Data.Verification.Address)
	}
	return &api.Userdata{
		FID:                    fid,
		Username:               currentUserdata.Username,
		CustodyAddress:         currentUserdata.CustodyAddress,
		VerificationsAddresses: verifications,
	}, nil
}

func (h *Hub) UserDataByVerificationAddress(ctx context.Context, address string) (*api.Userdata, error) {
	return nil, fmt.Errorf("not implemented")
}

func (h *Hub) newRequest(ctx context.Context, method string, uri string, body io.Reader) (*http.Request, error) {
	endpoint := fmt.Sprintf("%s/%s", h.endpoint, uri)
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}
	if h.auth != nil {
		for k, v := range h.auth {
			if k == "" || v == "" {
				continue
			}
			req.Header.Set(k, v)
		}
	}
	return req, nil
}
