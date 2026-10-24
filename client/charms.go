package client

import (
	"bytes"
	"context"
	"encoding/json"
)

// InstallCharmOptions identifies the charm archive to install.
type InstallCharmOptions struct {
	// SHA256 identifies the charm archive to install.
	SHA256 string
}

// InstallCharm creates a change that fetches and installs a charm.
func (client *Client) InstallCharm(options InstallCharmOptions) (string, error) {
	payload := struct {
		Action string `json:"action"`
		SHA256 string `json:"sha256"`
	}{
		Action: "install", SHA256: options.SHA256,
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
