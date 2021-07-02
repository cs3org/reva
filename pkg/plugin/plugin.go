package plugin

import (
	"os"
	"os/exec"

	"github.com/cs3org/reva/pkg/user"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-plugin"
)

// pluginMap contains all the plugins that can be consumed.
var pluginMap = map[string]plugin.Plugin{
	"userprovider": &user.UserProviderPlugin{},
}

var Handshake = plugin.HandshakeConfig{
	ProtocolVersion:  1,
	MagicCookieKey:   "BASIC_PLUGIN",
	MagicCookieValue: "hello",
}

func Load(driver string, pluginType string) (interface{}, error) {
	logger := hclog.New(&hclog.LoggerOptions{
		Name:   "plugin",
		Output: os.Stdout,
		Level:  hclog.Trace,
	})

	client := plugin.NewClient(&plugin.ClientConfig{
		HandshakeConfig: Handshake,
		Plugins:         pluginMap,
		Cmd:             exec.Command(driver),
		AllowedProtocols: []plugin.Protocol{
			plugin.ProtocolNetRPC,
		},
		Logger: logger,
	})

	rpcClient, err := client.Client()
	if err != nil {
		return nil, err
	}

	raw, err := rpcClient.Dispense(pluginType)
	if err != nil {
		return nil, err
	}

	return raw, nil
}
