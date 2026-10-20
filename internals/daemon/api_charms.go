package daemon

import (
	"encoding/json"
	"net/http"

	"github.com/SimonRichardson/charmd/internals/charmapi"
)

func v1PostCharms(command *Command, request *http.Request) Response {
	var payload struct {
		Action   string `json:"action"`
		Name     string `json:"name"`
		SHA256   string `json:"sha256"`
		Revision *int   `json:"revision"`
		Channel  string `json:"channel"`
	}
	if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
		return BadRequest("cannot decode request body: %v", err)
	}
	if payload.Action != "install" {
		return BadRequest("invalid action %q", payload.Action)
	}

	change, err := command.d.overlord.CharmManager().Install(charmapi.DownloadRequest{
		Name: payload.Name, SHA256: payload.SHA256, Revision: payload.Revision, Channel: payload.Channel,
	})
	if err != nil {
		return BadRequest("cannot install charm: %v", err)
	}
	return AsyncResponse(change.ID())
}
