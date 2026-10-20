package client

import (
	"bytes"
	"context"
	"encoding/json"
)

// InstallCharm creates a change that fetches and installs charm.
func (client *Client) InstallCharm(charm string) (string, error) {
	payload := struct {
		Action string `json:"action"`
		Charm  string `json:"charm"`
	}{
		Action: "install",
		Charm:  charm,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	resp, err := client.Requester().Do(context.Background(), &RequestOptions{
		Type:   AsyncRequest,
		Method: "POST",
		Path:   "/v1/charms",
		Body:   bytes.NewReader(data),
	})
	if err != nil {
		return "", err
	}
	return resp.ChangeID, nil
}
