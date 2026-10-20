package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/canonical/go-flags"
	"gopkg.in/yaml.v3"

	"github.com/SimonRichardson/charmd/client"
	"github.com/SimonRichardson/charmd/internals/charmapi"
	"github.com/SimonRichardson/charmd/internals/config"
	"github.com/SimonRichardson/charmd/internals/daemon"
	"github.com/SimonRichardson/charmd/internals/overlord"
	"github.com/SimonRichardson/charmd/internals/timeutil"
)

// Stdout and Stderr are the output streams used by command implementations.
var Stdout io.Writer = os.Stdout
var Stderr io.Writer = os.Stderr

const charmdEnv = "CHARMD"
const defaultCharmdDir = "/var/lib/charmd/default"

// Run is the command entry point used by the charmd binary.
func Run(args []string) error {
	if args == nil {
		args = os.Args[1:]
	}
	paths := runtimePaths()
	if len(args) > 0 {
		api, err := client.New(&client.Config{Socket: paths.socket})
		if err != nil {
			return err
		}
		return RunWithSocket(args, api, paths.socket, Stdout, Stderr)
	}
	return runDaemon(paths)
}

type runtimePathSet struct {
	dir    string
	config string
	socket string
	state  string
}

func runtimePaths() runtimePathSet {
	dir := defaultCharmdDir
	if configuredDir := os.Getenv(charmdEnv); configuredDir != "" {
		dir = configuredDir
	}
	return runtimePathSet{
		dir:    dir,
		config: filepath.Join(dir, "config.yaml"),
		socket: filepath.Join(dir, "charmd.sock"),
		state:  filepath.Join(dir, "state.json"),
	}
}

func runDaemon(paths runtimePathSet) error {
	runtimeConfig, err := config.Load(paths.config)
	if err != nil {
		return err
	}
	charmAPI, err := charmapi.New(runtimeConfig.Services.CharmAPI)
	if err != nil {
		return err
	}
	o, err := overlord.New(&overlord.Options{
		StatePath: paths.state,
		CharmAPI:  charmAPI,
	})
	if err != nil {
		return err
	}
	defer o.Stop()
	if err := o.StartUp(); err != nil {
		return err
	}
	if err := os.Remove(paths.socket); err != nil && !os.IsNotExist(err) {
		return err
	}
	listener, err := net.Listen("unix", paths.socket)
	if err != nil {
		return err
	}
	defer listener.Close()
	defer os.Remove(paths.socket)
	d := daemon.New(o)
	server := &http.Server{Handler: d.Handler(), ConnContext: daemon.PeerContext}
	defer server.Close()
	go server.Serve(listener)
	d.Start()
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	<-signals
	d.Stop()
	return nil
}

// ErrExtraArgs reports arguments not accepted by a command.
var ErrExtraArgs = errors.New("too many arguments")

// CmdOptions contains the client configuration supplied to a command.
type CmdOptions struct {
	Client     *client.Client
	SocketPath string
}

// CmdInfo describes a registered command.
type CmdInfo struct {
	Name, Summary, Description string
	ArgsHelp                   map[string]string
	New                        func(*CmdOptions) flags.Commander
}

var noticeCommands = map[string]*CmdInfo{}

// AddCommand registers a command.
func AddCommand(command *CmdInfo) { noticeCommands[command.Name] = command }

func merge(maps ...map[string]string) map[string]string {
	result := make(map[string]string)
	for _, m := range maps {
		for key, value := range m {
			result[key] = value
		}
	}
	return result
}

type timeMixin struct {
	AbsTime bool `long:"abs-time"`
}

var timeArgsHelp = map[string]string{"--abs-time": "Display absolute times"}

func (m timeMixin) fmtTime(t time.Time) string {
	if m.AbsTime {
		return t.Format(time.RFC3339)
	}
	return timeutil.Human(t)
}

type formatMixin struct {
	//lint:ignore SA5008 "choice" tag is intentionally duplicated
	Format string `long:"format" default:"text" choice:"text" choice:"json" choice:"yaml"`
}

var formatArgsHelp = map[string]string{"--format": "Output format"}

func (m formatMixin) formatNonText(result any) error {
	var data []byte
	var err error
	switch m.Format {
	case "json":
		data, err = json.Marshal(result)
		data = append(data, '\n')
	case "yaml":
		data, err = yaml.Marshal(result)
	default:
		return fmt.Errorf("invalid format %q", m.Format)
	}
	if err != nil {
		return err
	}
	_, err = Stdout.Write(data)
	return err
}

func tabWriter() *tabwriter.Writer { return tabwriter.NewWriter(Stdout, 5, 3, 2, ' ', 0) }

type cliState struct {
	NoticesLastListed  time.Time `json:"notices-last-listed"`
	NoticesLastOkayed  time.Time `json:"notices-last-okayed"`
	WarningsLastListed time.Time `json:"warnings-last-listed"`
	WarningsLastOkayed time.Time `json:"warnings-last-okayed"`
}

type fullCLIState struct {
	Charmd map[string]*cliState `json:"charmd"`
}

func cliStatePath() string {
	configDir := os.Getenv("XDG_CONFIG_HOME")
	if configDir == "" {
		configDir = os.ExpandEnv("$HOME/.config")
	}
	return filepath.Join(configDir, "charmd", "cli.json")
}

func loadFullCLIState() (*fullCLIState, error) {
	data, err := os.ReadFile(cliStatePath())
	if errors.Is(err, os.ErrNotExist) {
		return &fullCLIState{Charmd: make(map[string]*cliState)}, nil
	}
	if err != nil {
		return nil, err
	}
	var state fullCLIState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	if state.Charmd == nil {
		state.Charmd = make(map[string]*cliState)
	}
	return &state, nil
}

func loadCLIState(socket string) (*cliState, error) {
	state, err := loadFullCLIState()
	if err != nil {
		return nil, err
	}
	if entry := state.Charmd[socket]; entry != nil {
		return entry, nil
	}
	return &cliState{}, nil
}

func saveCLIState(socket string, state *cliState) error {
	all, err := loadFullCLIState()
	if err != nil {
		return err
	}
	all.Charmd[socket] = state
	data, err := json.Marshal(all)
	if err != nil {
		return err
	}
	path := cliStatePath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}
