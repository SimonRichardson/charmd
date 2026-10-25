package configapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"

	"github.com/SimonRichardson/charmd/internals/inotify"
	"github.com/SimonRichardson/charmd/internals/logger"
	"gopkg.in/yaml.v3"
)

// fileClient implements the service contract against a YAML file keyed by charm checksum.
type fileClient struct{ path string }

// read loads a complete snapshot without publishing malformed or partial data.
func (c *fileClient) read() (map[string]map[string]any, error) {
	file, err := os.Open(c.path)
	if err != nil {
		return nil, fmt.Errorf("open config file: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() > 4*1024*1024 {
		return nil, fmt.Errorf("config file exceeds size limit")
	}
	var data struct {
		Charms map[string]map[string]any `yaml:"charms"`
	}
	decoder := yaml.NewDecoder(io.LimitReader(file, 4*1024*1024+1))
	decoder.KnownFields(true)
	if err := decoder.Decode(&data); err != nil {
		return nil, fmt.Errorf("decode config file: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("config file must contain one YAML document")
	}
	for checksum, values := range data.Charms {
		if err := validateChecksum(checksum); err != nil {
			return nil, err
		}
		if values == nil {
			data.Charms[checksum] = map[string]any{}
		}
	}
	// Normalize to the same JSON value types as the remote service, rejecting
	// YAML values that could not be persisted on a hook task.
	encoded, err := json.Marshal(data.Charms)
	if err != nil {
		return nil, fmt.Errorf("config file must contain JSON-compatible values: %w", err)
	}
	var snapshot map[string]map[string]any
	if err := json.Unmarshal(encoded, &snapshot); err != nil {
		return nil, err
	}
	return snapshot, nil
}

// Get reads the current overrides. Removing a charm entry restores all defaults.
func (c *fileClient) Get(ctx context.Context, checksum string) (map[string]any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateChecksum(checksum); err != nil {
		return nil, err
	}
	values, err := c.read()
	if err != nil {
		return nil, err
	}
	if result := values[checksum]; result != nil {
		return result, nil
	}
	return map[string]any{}, nil
}

// Watch publishes initial entries, then semantic changes only. Invalid updates
// leave the last good snapshot intact so a later repair can be compared safely.
func (c *fileClient) Watch(ctx context.Context, notify func(string)) error {
	var previous map[string]map[string]any
	initial := true
	return inotify.WatchFile(ctx, c.path, func() error {
		next, err := c.read()
		if err != nil {
			if initial {
				return err
			}
			logger.Noticef("config API file update: %v", err)
			return nil
		}
		for checksum, values := range next {
			if old, exists := previous[checksum]; !exists || !reflect.DeepEqual(old, values) {
				notify(checksum)
			}
		}
		for checksum := range previous {
			if _, exists := next[checksum]; !exists {
				notify(checksum)
			}
		}
		previous, initial = next, false
		return nil
	})
}
