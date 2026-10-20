package daemon

import (
	"encoding/json"
	"net/http"
	"slices"
	"time"

	"github.com/SimonRichardson/charmd/internals/overlord/state"
)

type changeInfo struct {
	ID      string      `json:"id"`
	Kind    string      `json:"kind"`
	Summary string      `json:"summary"`
	Status  string      `json:"status"`
	Tasks   []*taskInfo `json:"tasks,omitempty"`
	Ready   bool        `json:"ready"`
	Err     string      `json:"err,omitempty"`

	SpawnTime time.Time                   `json:"spawn-time"`
	ReadyTime *time.Time                  `json:"ready-time,omitempty"`
	Data      map[string]*json.RawMessage `json:"data,omitempty"`
}

type taskInfo struct {
	ID       string           `json:"id"`
	Kind     string           `json:"kind"`
	Summary  string           `json:"summary"`
	Status   string           `json:"status"`
	Log      []string         `json:"log,omitempty"`
	Progress taskInfoProgress `json:"progress"`

	SpawnTime time.Time                   `json:"spawn-time"`
	ReadyTime *time.Time                  `json:"ready-time,omitempty"`
	Data      map[string]*json.RawMessage `json:"data,omitempty"`
}

type taskInfoProgress struct {
	Label string `json:"label"`
	Done  int    `json:"done"`
	Total int    `json:"total"`
}

func changeToAPI(change *state.Change) *changeInfo {
	status := change.Status()
	result := &changeInfo{
		ID:        change.ID(),
		Kind:      change.Kind(),
		Summary:   change.Summary(),
		Status:    status.String(),
		Ready:     status.Ready(),
		SpawnTime: change.SpawnTime(),
	}
	if readyTime := change.ReadyTime(); !readyTime.IsZero() {
		result.ReadyTime = &readyTime
	}
	if err := change.Err(); err != nil {
		result.Err = err.Error()
	}
	for _, task := range change.Tasks() {
		label, done, total := task.Progress()
		taskResult := &taskInfo{
			ID:        task.ID(),
			Kind:      task.Kind(),
			Summary:   task.Summary(),
			Status:    task.Status().String(),
			Log:       task.Log(),
			Progress:  taskInfoProgress{Label: label, Done: done, Total: total},
			SpawnTime: task.SpawnTime(),
		}
		if readyTime := task.ReadyTime(); !readyTime.IsZero() {
			taskResult.ReadyTime = &readyTime
		}
		_ = task.Get("api-data", &taskResult.Data)
		result.Tasks = append(result.Tasks, taskResult)
	}
	_ = change.Get("api-data", &result.Data)
	return result
}

func v1GetChanges(command *Command, request *http.Request) Response {
	selector := request.URL.Query().Get("select")
	if selector == "" {
		selector = "in-progress"
	}
	if selector != "all" && selector != "ready" && selector != "in-progress" {
		return BadRequest("select should be one of: all,in-progress,ready")
	}
	service := request.URL.Query().Get("for")
	st := command.d.overlord.State()
	st.Lock()
	defer st.Unlock()
	var result []*changeInfo
	for _, change := range st.Changes() {
		ready := change.Status().Ready()
		if selector != "all" && (selector != "ready" || !ready) && (selector != "in-progress" || ready) {
			continue
		}
		if service != "" {
			var services []string
			if err := change.Get("service-names", &services); err != nil || !slices.Contains(services, service) {
				continue
			}
		}
		result = append(result, changeToAPI(change))
	}
	return SyncResponse(result)
}

func v1GetChange(command *Command, request *http.Request) Response {
	id := request.PathValue("id")
	st := command.d.overlord.State()
	st.Lock()
	defer st.Unlock()
	change := st.Change(id)
	if change == nil {
		return NotFound("cannot find change with id %q", id)
	}
	return SyncResponse(changeToAPI(change))
}

func v1GetChangeWait(command *Command, request *http.Request) Response {
	id := request.PathValue("id")
	st := command.d.overlord.State()
	st.Lock()
	change := st.Change(id)
	st.Unlock()
	if change == nil {
		return NotFound("cannot find change with id %q", id)
	}
	timeout, err := parseOptionalDuration(request.URL.Query().Get("timeout"))
	if err != nil {
		return BadRequest("invalid timeout: %v", err)
	}
	if timeout == 0 {
		select {
		case <-change.Ready():
		case <-request.Context().Done():
			return ServerError("request canceled")
		}
	} else {
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		select {
		case <-change.Ready():
		case <-timer.C:
			return GatewayTimeout("timed out waiting for change after %s", timeout)
		case <-request.Context().Done():
			return ServerError("request canceled")
		}
	}
	st.Lock()
	defer st.Unlock()
	return SyncResponse(changeToAPI(change))
}

func v1PostChange(command *Command, request *http.Request) Response {
	id := request.PathValue("id")
	st := command.d.overlord.State()
	st.Lock()
	defer st.Unlock()
	change := st.Change(id)
	if change == nil {
		return NotFound("cannot find change with id %q", id)
	}
	var payload struct {
		Action string `json:"action"`
	}
	if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
		return BadRequest("cannot decode request body: %v", err)
	}
	if payload.Action != "abort" {
		return BadRequest("invalid action %q", payload.Action)
	}
	if change.Status().Ready() {
		return BadRequest("cannot abort change %s with nothing pending", id)
	}
	change.Abort()
	st.EnsureBefore(0)
	return SyncResponse(changeToAPI(change))
}
