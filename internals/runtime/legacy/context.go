package legacy

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	charmRuntime "github.com/SimonRichardson/charmd/internals/runtime"
	"github.com/SimonRichardson/charmd/internals/runtime/legacy/hookd"
	"gopkg.in/yaml.v3"
)

// Context is the runner's view of a hook invocation. It provides the hook-tool
// operations together with environment preparation and result collection.
type Context interface {
	hookd.Context
	// Environment returns the process environment for the hook and invocation paths.
	Environment(Paths, string) []string
	// Result snapshots the hook changes that should be persisted.
	Result() charmRuntime.Result
}

// HookContext holds the state visible to hook tools during one invocation.
// It owns charm data and results, independently of RPC and process execution.
type HookContext struct {
	mu     sync.Mutex
	leader bool
	id     string
	name   string
	config map[string]any
	status *charmRuntime.Status
	log    io.Writer
}

// NewHookContext loads charm metadata and config defaults and allocates a fresh invocation ID.
func NewHookContext(dir string, log io.Writer) (*HookContext, error) {
	metadata, err := os.ReadFile(filepath.Join(dir, "metadata.yaml"))
	if err != nil {
		return nil, fmt.Errorf("read charm metadata: %w", err)
	}
	var meta struct {
		Name string `yaml:"name"`
	}
	if err := yaml.Unmarshal(metadata, &meta); err != nil {
		return nil, fmt.Errorf("read charm metadata: %w", err)
	}
	if meta.Name == "" {
		return nil, fmt.Errorf("charm metadata has no name")
	}
	config, err := configDefaults(dir)
	if err != nil {
		return nil, err
	}
	token := make([]byte, 32)
	if _, err := rand.Read(token); err != nil {
		return nil, err
	}
	return &HookContext{id: hex.EncodeToString(token), name: meta.Name, config: config, log: log}, nil
}

// ID returns the unique identifier used to authenticate this hook invocation.
func (c *HookContext) ID() string { return c.id }

// Config returns configuration defaults loaded for this invocation. Callers must not modify the map.
func (c *HookContext) Config() map[string]any { return c.config }

// SetStatus replaces the invocation workload status under the context lock.
func (c *HookContext) SetStatus(status charmRuntime.Status) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.status = &status
}

// Status returns a snapshot of the workload status, defaulting to unknown.
func (c *HookContext) Status() charmRuntime.Status {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.status == nil {
		return charmRuntime.Status{Status: "unknown"}
	}
	return *c.status
}

// Log writes a hook-tool message to the invocation log destination.
func (c *HookContext) Log(message string) error {
	if c.log == nil {
		return nil
	}
	_, err := fmt.Fprintln(c.log, message)
	return err
}

// Result returns an independent snapshot of hook changes for persistence.
func (c *HookContext) Result() charmRuntime.Result {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.status == nil {
		return charmRuntime.Result{}
	}
	status := *c.status
	return charmRuntime.Result{Status: &status}
}

// configDefaults loads option defaults, treating an absent config file as an empty configuration.
func configDefaults(dir string) (map[string]any, error) {
	data, err := os.ReadFile(filepath.Join(dir, "config.yaml"))
	if os.IsNotExist(err) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}
	var config struct {
		Options map[string]struct {
			Default any `yaml:"default"`
		} `yaml:"options"`
	}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("read charm config: %w", err)
	}
	values := make(map[string]any, len(config.Options))
	for key, option := range config.Options {
		values[key] = option.Default
	}
	return values, nil
}

// applyConfig overlays an invocation snapshot without modifying the caller's map or charm defaults.
// Unknown options fail before any charm code executes.
func (c *HookContext) applyConfig(values map[string]any) error {
	for name := range values {
		if _, ok := c.config[name]; !ok {
			return fmt.Errorf("unknown charm config option %q", name)
		}
	}
	// Copy nested JSON values as well as the outer map so later invocations
	// cannot mutate the snapshot visible to this hook.
	data, err := json.Marshal(values)
	if err != nil {
		return fmt.Errorf("copy charm config: %w", err)
	}
	var snapshot map[string]any
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return err
	}
	for name, value := range snapshot {
		c.config[name] = value
	}
	return nil
}

// IsLeader returns the leadership snapshot fixed before the hook started.
func (c *HookContext) IsLeader() bool { return c.leader }
