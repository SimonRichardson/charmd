package cli

import (
	"github.com/SimonRichardson/charmd/client"
	"github.com/canonical/go-flags"
)

var testClient *client.Client
var testSocket string

// SetNoticeTestClient configures the upstream command test fixture.
func SetNoticeTestClient(api *client.Client, socket string) {
	testClient, testSocket = api, socket
}

// ParserForTest returns a parser for the retained upstream command tests.
func ParserForTest() *flags.Parser {
	parser := flags.NewParser(nil, flags.None)
	for _, name := range []string{"notice", "notices", "okay"} {
		command := noticeCommands[name]
		_, err := parser.AddCommand(name, command.Summary, command.Description,
			command.New(&CmdOptions{Client: testClient, SocketPath: testSocket}))
		if err != nil {
			panic(err)
		}
	}
	return parser
}
