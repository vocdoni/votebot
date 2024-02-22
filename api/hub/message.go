package hub

const (
	MESSAGE_TYPE_CAST_ADD = "MESSAGE_TYPE_CAST_ADD"
)

type HubCastAddBody struct {
	Text      string `json:"text"`
	ParentURL string `json:"parentUrl"`
}

type HubMessageData struct {
	Type        string          `json:"type"`
	From        uint64          `json:"fid"`
	Timestamp   uint64          `json:"timestamp"`
	CastAddBody *HubCastAddBody `json:"castAddBody,omitempty"`
}

type HubMessage struct {
	Data    *HubMessageData `json:"data"`
	HexHash string          `json:"hash"`
}

type HubMentionsResponse struct {
	Messages []*HubMessage `json:"messages"`
}
