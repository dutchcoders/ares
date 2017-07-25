package server

import (
	"encoding/json"
	"fmt"
	_ "log"
	"net/http"
	"os"

	"bytes"
	"io"

	"github.com/nlopes/slack"
)

// Sends a message to the Slack channel about the Event.
func send_message(params slack.PostMessageParameters) error {
	webhookURL := "https://hooks.slack.com/services/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"
	if webhookURL == "" {
		return fmt.Errorf("WEBHOOK_URL not set.")
	}

	buffer := new(bytes.Buffer)
	if err := json.NewEncoder(buffer).Encode(params); err != nil {
		return err
	}

	var req *http.Request
	if r, err := http.NewRequest("POST", webhookURL, io.TeeReader(buffer, os.Stdout)); err != nil {
		return err
	} else {
		req = r
	}

	req.Header.Set("Content-Type", "application/json")
	if resp, err := http.DefaultClient.Do(req); err != nil {
		return err
	} else {
		resp.Body.Close()
	}

	return nil
}
