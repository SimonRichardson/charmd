package client

import (
	"bytes"
	"context"
	"encoding/json"
)

// InstallCharmOptions identifies the charm archive to install.
type InstallCharmOptions struct {
	// Name is the name of the charm to install.
	Name string
	// SHA256 optionally constrains Charmhub and is required by Juju.
	SHA256 string
	// Revision optionally selects a Charmhub revision.
	Revision *int
	// Channel optionally selects a Charmhub channel.
	Channel string
}

// InstallCharm creates a change that fetches and installs a charm.
func (client *Client) InstallCharm(options InstallCharmOptions) (string, error) {
	payload := struct {
		Action   string `json:"action"`
		Name     string `json:"name"`
		SHA256   string `json:"sha256,omitempty"`
		Revision *int   `json:"revision,omitempty"`
		Channel  string `json:"channel,omitempty"`
	}{
		Action: "install", Name: options.Name, SHA256: options.SHA256, Revision: options.Revision, Channel: options.Channel,
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
