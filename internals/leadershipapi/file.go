package leadershipapi

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/SimonRichardson/charmd/internals/inotify"
	"github.com/SimonRichardson/charmd/internals/logger"
	"gopkg.in/yaml.v3"
)

// fileClient implements the service contract against a YAML file keyed by charm checksum.
type fileClient struct{ path string }

// read loads a complete snapshot without publishing malformed or partial data.
func (c *fileClient) read() (map[string]bool, error) {
	file, err := os.Open(c.path)
	if err != nil {
		return nil, fmt.Errorf("open leadership file: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() > 4*1024*1024 {
		return nil, fmt.Errorf("leadership file exceeds size limit")
	}
	var data struct {
		Charms map[string]*bool `yaml:"charms"`
	}
	decoder := yaml.NewDecoder(io.LimitReader(file, 4*1024*1024+1))
	decoder.KnownFields(true)
	if err := decoder.Decode(&data); err != nil {
		return nil, fmt.Errorf("decode leadership file: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("leadership file must contain one YAML document")
	}
	snapshot := make(map[string]bool, len(data.Charms))
	for checksum, leader := range data.Charms {
		if leader == nil {
			return nil, fmt.Errorf("leadership for %s must be a boolean", checksum)
		}
		snapshot[checksum] = *leader
		if err := validateChecksum(checksum); err != nil {
			return nil, err
		}
	}
	return snapshot, nil
}

// Get reads current unit leadership; an absent charm entry means non-leader.
func (c *fileClient) Get(ctx context.Context, checksum string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if err := validateChecksum(checksum); err != nil {
		return false, err
	}
	values, err := c.read()
	if err != nil {
		return false, err
	}
	return values[checksum], nil
}

// Watch publishes initial entries, then semantic changes only. Invalid updates
// leave the last good snapshot intact so a later repair can be compared safely.
func (c *fileClient) Watch(ctx context.Context, notify func(string)) error {
	var previous map[string]bool
	initial := true
	return inotify.WatchFile(ctx, c.path, func() error {
		next, err := c.read()
		if err != nil {
			if initial {
				return err
			}
			logger.Noticef("leadership API file update: %v", err)
			return nil
		}
		for checksum, values := range next {
			if old, exists := previous[checksum]; !exists || old != values {
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
