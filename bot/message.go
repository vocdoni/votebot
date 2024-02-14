package bot

const (
	MESSAGE_TYPE_CAST_ADD = "MESSAGE_TYPE_CAST_ADD"
)

type CastAddBody struct {
	Text      string `json:"text"`
	ParentURL string `json:"parentUrl"`
}

type MessageData struct {
	Type        string       `json:"type"`
	From        uint64       `json:"fid"`
	Timestamp   uint64       `json:"timestamp"`
	CastAddBody *CastAddBody `json:"castAddBody,omitempty"`
}

type Message struct {
	Data *MessageData `json:"data"`
}

func (m *Message) IsMention() bool {
	return m.Data.Type == MESSAGE_TYPE_CAST_ADD && m.Data.CastAddBody != nil && m.Data.CastAddBody.Text != ""
}

func (m *Message) Mention() string {
	return m.Data.CastAddBody.Text
}

func (m *Message) ParentURL() string {
	return m.Data.CastAddBody.ParentURL
}
