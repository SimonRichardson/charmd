package overlord

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

// prepareCheckpoint translates the previous charmd checkpoint schema to the
// Charmd schema before it is loaded. It never changes the original file.
func prepareCheckpoint(data []byte) ([]byte, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil || root == nil {
		return nil, fmt.Errorf("cannot read state: expected a valid JSON object")
	}
	legacyLast, legacy := root["last-id"]
	if !legacy {
		return data, nil
	}
	var lastID int
	if err := json.Unmarshal(legacyLast, &lastID); err != nil || lastID < 0 {
		return nil, fmt.Errorf("cannot read state: invalid legacy ID counter")
	}
	type legacyTask struct {
		ID        string    `json:"id"`
		Status    int       `json:"status"`
		Change    string    `json:"change"`
		WaitTasks []string  `json:"wait-tasks"`
		Error     string    `json:"error"`
		SpawnTime time.Time `json:"spawn-time"`
	}
	type legacyChange struct {
		ID      string   `json:"id"`
		TaskIDs []string `json:"task-ids"`
	}
	var rawTasks, rawChanges map[string]json.RawMessage
	if err := json.Unmarshal(root["tasks"], &rawTasks); err != nil {
		return nil, fmt.Errorf("cannot read legacy tasks: %w", err)
	}
	if err := json.Unmarshal(root["changes"], &rawChanges); err != nil {
		return nil, fmt.Errorf("cannot read legacy changes: %w", err)
	}
	if rawTasks == nil {
		rawTasks = make(map[string]json.RawMessage)
	}
	if rawChanges == nil {
		rawChanges = make(map[string]json.RawMessage)
	}
	tasks := make(map[string]legacyTask, len(rawTasks))
	changes := make(map[string]legacyChange, len(rawChanges))
	validID := func(id, actual string) bool {
		n, err := strconv.Atoi(id)
		return err == nil && n > 0 && n <= lastID && id == actual
	}
	for id, raw := range rawTasks {
		var task legacyTask
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) || json.Unmarshal(raw, &task) != nil ||
			!validID(id, task.ID) || task.Status < 0 || task.Status > 5 {
			return nil, fmt.Errorf("cannot read legacy task %q", id)
		}
		tasks[id] = task
	}
	for id, raw := range rawChanges {
		var change legacyChange
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) || json.Unmarshal(raw, &change) != nil ||
			!validID(id, change.ID) {
			return nil, fmt.Errorf("cannot read legacy change %q", id)
		}
		changes[id] = change
	}
	owners := make(map[string]string)
	for id, change := range changes {
		for _, taskID := range change.TaskIDs {
			task, exists := tasks[taskID]
			if !exists || task.Change != id || owners[taskID] != "" {
				return nil, fmt.Errorf("invalid task %q in legacy change %q", taskID, id)
			}
			owners[taskID] = id
		}
	}
	halts := make(map[string][]string)
	for id, task := range tasks {
		if task.Change != owners[id] {
			return nil, fmt.Errorf("invalid legacy task ownership for %q", id)
		}
		for _, dependency := range task.WaitTasks {
			if _, exists := tasks[dependency]; !exists {
				return nil, fmt.Errorf("legacy task %q references missing task %q", id, dependency)
			}
			halts[dependency] = append(halts[dependency], id)
		}
	}
	visiting := make(map[string]bool)
	visited := make(map[string]bool)
	var checkCycle func(string) bool
	checkCycle = func(id string) bool {
		if visiting[id] {
			return true
		}
		if visited[id] {
			return false
		}
		visiting[id] = true
		for _, dependency := range tasks[id].WaitTasks {
			if checkCycle(dependency) {
				return true
			}
		}
		visiting[id] = false
		visited[id] = true
		return false
	}
	for id := range tasks {
		if checkCycle(id) {
			return nil, fmt.Errorf("legacy task dependency cycle involving %q", id)
		}
	}
	for id, task := range tasks {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(rawTasks[id], &fields); err != nil {
			return nil, err
		}
		if task.Status == 5 {
			fields["status"] = json.RawMessage("9") // Charmd's ErrorStatus.
		}
		if task.Error != "" {
			log, _ := json.Marshal([]string{task.SpawnTime.Format(time.RFC3339) + " ERROR " + task.Error})
			fields["log"] = log
		}
		if len(halts[id]) > 0 {
			fields["halt-tasks"], _ = json.Marshal(halts[id])
		}
		rawTasks[id], _ = json.Marshal(fields)
	}
	root["tasks"], _ = json.Marshal(rawTasks)
	root["changes"], _ = json.Marshal(rawChanges)
	root["last-change-id"] = legacyLast
	root["last-task-id"] = legacyLast
	root["data"] = json.RawMessage(`{}`)
	delete(root, "last-id")
	return json.Marshal(root)
}
