package bot

import "fmt"

var (
	ErrAPINotSet            = fmt.Errorf("api not set")
	ErrBotFIDNotSet         = fmt.Errorf("bot fid not set")
	ErrPrivateKeyNotSet     = fmt.Errorf("private key not set")
	ErrDecodingPrivateKey   = fmt.Errorf("error decoding provided private key")
	ErrEndpointNotSet       = fmt.Errorf("endpoint not set")
	ErrUnrecognisedCommand  = fmt.Errorf("unrecognised command")
	ErrQuestionNotSet       = fmt.Errorf("question content not set")
	ErrParsingDuration      = fmt.Errorf("error parsing duration")
	ErrMinOptionsNotReached = fmt.Errorf("min number of options not reached")
	ErrMaxOptionsReached    = fmt.Errorf("max number of options reached")
	ErrNoNewCasts           = fmt.Errorf("no new casts")
)
