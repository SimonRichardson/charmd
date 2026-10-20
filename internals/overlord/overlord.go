package overlord

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/SimonRichardson/charmd/internals/charmapi"
	"github.com/SimonRichardson/charmd/internals/overlord/charmstate"
	"github.com/SimonRichardson/charmd/internals/overlord/state"
)

// Options supplies the static dependencies used to construct an Overlord.
type Options struct {
	// StatePath is the path to the persistent state checkpoint.
	StatePath string
	// CharmAPI accesses the Juju-provided charm API.
	CharmAPI *charmapi.Client
}

// Overlord owns the sole State and coordinates managers and the shared task runner.
type Overlord struct {
	state       *state.State
	runner      *state.TaskRunner
	engine      *StateEngine
	charmMgr    *charmstate.Manager
	wake        chan struct{}
	stop        chan struct{}
	done        chan struct{}
	once        sync.Once
	lifecycleMu sync.Mutex
	started     bool
	stopped     bool
	runnerAdded bool
	scheduleMu  sync.Mutex
	nextEnsure  time.Time
}

// New loads file-backed state and constructs its core managers without starting them.
func New(opts *Options) (*Overlord, error) {
	if opts == nil {
		return nil, fmt.Errorf("overlord options are required")
	}
	if opts.StatePath == "" {
		return nil, fmt.Errorf("state path is required")
	}
	if opts.CharmAPI == nil {
		return nil, fmt.Errorf("charm API client is required")
	}
	o := &Overlord{
		wake: make(chan struct{}, 1),
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}
	backend := &fileBackend{path: opts.StatePath, ensure: o.EnsureBefore}
	data, err := os.ReadFile(opts.StatePath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("load state: %w", err)
		}
		o.state = state.New(backend)
	} else {
		o.state, err = state.ReadState(backend, bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
	}
	o.engine = NewStateEngine(o.state)
	o.runner = state.NewTaskRunner(o.state)
	o.charmMgr = charmstate.NewManager(o.state, opts.CharmAPI)
	o.engine.AddManager(o.charmMgr)
	return o, nil
}

// State returns the overlord's only State. Callers must use its Lock/Unlock pair.
func (o *Overlord) State() *state.State { return o.state }

// TaskRunner returns the shared task dispatcher.
func (o *Overlord) TaskRunner() *state.TaskRunner { return o.runner }

// CharmManager returns the manager responsible for charm changes.
func (o *Overlord) CharmManager() *charmstate.Manager { return o.charmMgr }

// AddManager registers a manager before StartUp. It panics after startup begins.
func (o *Overlord) AddManager(m StateManager) {
	o.lifecycleMu.Lock()
	defer o.lifecycleMu.Unlock()
	if o.runnerAdded || o.stopped {
		panic("cannot add a manager after overlord startup or stop")
	}
	o.engine.AddManager(m)
}

// EnsureBefore schedules an ensure pass no later than d from now.
func (o *Overlord) EnsureBefore(d time.Duration) {
	when := time.Now().Add(d)
	o.scheduleMu.Lock()
	if o.nextEnsure.IsZero() || when.Before(o.nextEnsure) {
		o.nextEnsure = when
	}
	o.scheduleMu.Unlock()
	select {
	case o.wake <- struct{}{}:
	default:
	}
}

// Ensure runs managers and schedules all currently runnable tasks.
func (o *Overlord) Ensure() error {
	return o.engine.Ensure()
}

// StartUp initializes managers and starts the asynchronous ensure loop once.
func (o *Overlord) StartUp() error {
	o.lifecycleMu.Lock()
	defer o.lifecycleMu.Unlock()
	if o.stopped {
		return fmt.Errorf("overlord is stopped")
	}
	if o.started {
		return nil
	}
	if !o.runnerAdded {
		o.engine.AddManager(o.runner)
		o.runnerAdded = true
	}
	if err := o.engine.StartUp(); err != nil {
		return err
	}
	o.started = true
	go o.loop()
	o.EnsureBefore(0)
	return nil
}

func (o *Overlord) loop() {
	defer close(o.done)
	const ensureInterval = 5 * time.Minute
	timer := time.NewTimer(ensureInterval)
	defer timer.Stop()
	next := time.Now().Add(ensureInterval)
	for {
		select {
		case <-o.wake:
			o.scheduleMu.Lock()
			if !o.nextEnsure.IsZero() && o.nextEnsure.Before(next) {
				next = o.nextEnsure
				timer.Reset(time.Until(next))
			}
			o.nextEnsure = time.Time{}
			o.scheduleMu.Unlock()
		case <-timer.C:
			if err := o.Ensure(); err != nil {
				log.Printf("state ensure error: %v", err)
			}
			next = time.Now().Add(ensureInterval)
			timer.Reset(ensureInterval)
		case <-o.stop:
			return
		}
	}
}

// Stop prevents new ensure passes, waits for active handlers, then checkpoints
// their committed terminal state. It is safe to call before StartUp.
func (o *Overlord) Stop() {
	o.once.Do(func() {
		o.lifecycleMu.Lock()
		o.stopped = true
		started := o.started
		close(o.stop)
		o.lifecycleMu.Unlock()
		if started {
			<-o.done
		}
		if !o.runnerAdded {
			o.runner.Stop()
		}
		o.engine.Stop()
		o.state.Lock()
		o.state.Unlock()
	})
}
