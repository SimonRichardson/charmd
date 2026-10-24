package daemon

import (
	"encoding/json"
	"net/http"
)

func v1PostCharms(command *Command, request *http.Request) Response {
	var payload struct {
		Action string `json:"action"`
		SHA256 string `json:"sha256"`
	}
	if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
		return BadRequest("cannot decode request body: %v", err)
	}
	if payload.Action != "install" {
		return BadRequest("invalid action %q", payload.Action)
	}

	change, err := command.d.overlord.CharmManager().Install(payload.SHA256)
	if err != nil {
		return BadRequest("cannot install charm: %v", err)
	}
	return AsyncResponse(change.ID())
}
