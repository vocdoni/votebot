package election

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	// farcaster.vote endpoints
	createEndpoint = "create"
	checkEndpoint  = "create/check/%s"
	// timeouts
	createTimeout = 10 * time.Second
	checkTimeout  = 10 * time.Second
)

type Profile struct {
	FID           uint64   `json:"fid"`
	Custody       string   `json:"custody"`
	Verifications []string `json:"verifications"`
}

type ElectionOptions struct {
	BaseEndpoint string   `json:"-"`
	Author       *Profile `json:"profile"`
	Question     string   `json:"question"`
	Options      []string `json:"options"`
	Duration     int      `json:"duration"`
}

// FrameElection creates a new election frame and returns the url to interact
// with it. It requests the creation of the election frame and then checks
// until the election frame is created. It returns the url when the election
// is created or an error if something goes wrong.
func FrameElection(ctx context.Context, opts *ElectionOptions) (string, error) {
	// create internal context
	createCtx, cancelCreate := context.WithTimeout(ctx, createTimeout)
	defer cancelCreate()
	// marshal the options
	body, err := json.Marshal(opts)
	if err != nil {
		return "", fmt.Errorf("error marshaling the election options: %w", err)
	}
	// create the election request and launch the creation process
	createURL := fmt.Sprintf("%s/%s", opts.BaseEndpoint, createEndpoint)
	req, err := http.NewRequestWithContext(createCtx, http.MethodPost, createURL, bytes.NewBuffer(body))
	if err != nil {
		return "", fmt.Errorf("error creating the election request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("error creating the election: %w", err)
	}
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("error creating the election: %s", res.Status)
	}
	// read the election id
	electionID, err := io.ReadAll(res.Body)
	if err != nil {
		return "", fmt.Errorf("error reading election id during the creation: %w", err)
	}
	// request the check until response status will be different from 204
	checkCtx, cancelCheck := context.WithTimeout(ctx, checkTimeout)
	defer cancelCheck()
	checkBaseURL := fmt.Sprintf("%s/%s", opts.BaseEndpoint, checkEndpoint)
	checkURL := fmt.Sprintf(checkBaseURL, strings.TrimSpace(string(electionID)))
	checkReq, err := http.NewRequestWithContext(checkCtx, http.MethodGet, checkURL, nil)
	if err != nil {
		return "", fmt.Errorf("error creating the check request: %w", err)
	}
	var checkRes *http.Response
	for {
		checkRes, err = http.DefaultClient.Do(checkReq)
		// if the status is 204, wait 1 second and check again
		if checkRes.StatusCode == http.StatusNoContent {
			time.Sleep(1 * time.Second)
			continue
		}
		// if the status is different from 204, and the error is different from
		// nil, return the error
		if err != nil {
			return "", fmt.Errorf("error checking the election: %w", err)
		}
		// if the status is different from 204 and there is no error, break
		// the loop
		break
	}
	// if the status is different from 200, return the error
	if checkRes.StatusCode != http.StatusOK {
		return "", fmt.Errorf("error checking the election: %s", checkRes.Status)
	}
	// if the status is 200, the election has been created, compose the url and return it
	return fmt.Sprintf("%s/%s", opts.BaseEndpoint, electionID), nil
}
