package plugin

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/cs3org/reva/pkg/user"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-plugin"
)

var pluginMap = map[string]plugin.Plugin{
	"json": &user.JSONPlugin{},
}

var Handshake = plugin.HandshakeConfig{
	ProtocolVersion:  1,
	MagicCookieKey:   "BASIC_PLUGIN",
	MagicCookieValue: "hello",
}

// for the time being, I'm assuming the location to be the location of the binary
func Load(location string) (interface{}, error) {
	logger := hclog.New(&hclog.LoggerOptions{
		Name:   "plugin",
		Output: os.Stdout,
		Level:  hclog.Trace,
	})

	client := plugin.NewClient(&plugin.ClientConfig{
		HandshakeConfig: Handshake,
		Plugins:         pluginMap,
		Cmd:             exec.Command(location),
		AllowedProtocols: []plugin.Protocol{
			plugin.ProtocolNetRPC,
		},
		Logger: logger,
	})

	//	defer client.Kill()

	rpcClient, err := client.Client()
	if err != nil {
		return nil, err
	}

	raw, err := rpcClient.Dispense("json")
	if err != nil {
		return nil, err
	}

	fmt.Println("PLUGIN: plugin loaded")
	return raw, nil
}
