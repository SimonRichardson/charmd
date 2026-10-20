package daemon

import (
	"encoding/json"
	"net/http"
)

func v1PostCharms(command *Command, request *http.Request) Response {
	var payload struct {
		Action string `json:"action"`
		Charm  string `json:"charm"`
	}
	if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
		return BadRequest("cannot decode request body: %v", err)
	}
	if payload.Action != "install" {
		return BadRequest("invalid action %q", payload.Action)
	}

	change, err := command.d.overlord.CharmManager().Install(payload.Charm)
	if err != nil {
		return BadRequest("cannot install charm: %v", err)
	}
	return AsyncResponse(change.ID())
}
