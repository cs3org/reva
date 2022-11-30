---
title: "Runtime Plugins"
linkTitle: "Runtime Plugins"
weight: 10
description: >
  Guide for developing runtime plugin drivers for Reva
---

## Reva Plugins

Reva Plugins allow new functionality to be added to Reva without modifying the core source code. Reva Plugins are able to add new custom drivers for various services to Reva and also at the same time enable loading existing plugins at runtime.

This page serves as a guide for understanding and developing Reva plugins.

### How Plugins Work

A Reva plugin represents a *service driver*.

Reva plugins are completely seperate, standalone applications that the core of Reva starts and communicates with. These plugin applications aren't meant to be run manually. Instead, Reva launches and communicates with them. 

Reva uses hashicorp's [go-plugin](https://github.com/hashicorp/go-plugin) framework to implement the plugin architecture. The plugin processes communicate with the Core using the go-plugin framework, which internally uses RPCs for the communication. The Reva core itself is responsible for launching and cleaning up the plugin processes.

### Developing Plugins

Reva plugins must be written in [Go](https://golang.org/), so you should be familiar with the language.

As mentioned earlier, the components that can be created and used in a Reva Plugin are service drivers. These drivers can belong to any of the Reva service, eg: Userprovider, Storageprovider, Authprovider etc. Each service exposes an [interface](https://golang.org/doc/effective_go#interfaces_and_types) for the plugin to implement.

All you need to do to create a plugin is:

1. Create an implementation of the desired interface: A Plugin should implement the interface exposed by the corresponding service. Eg: [Userprovider](https://github.com/cs3org/reva/blob/master/pkg/user/user.go#L67) interface.
2. Serve the plugin using the [go-plugin's](https://github.com/hashicorp/go-plugin) `plugin.Serve` method.

The core handles all of the communication details and go-plugin implementations inside the server.

Your plugin must use the packages from the Reva core to implement the interfaces. You're encouraged to use whatever other packages you want in your plugin implementation. Because plugins are their own processes, there is no danger of colliding dependencies.

- `github.com/cs3org/reva/v2/pkg/<service_type>`: Contains the interface that you have to implement for any give plugin.
- `github.com/hashicorp/go-plugin`: To serve the plugin over RPC. This handles all the inter-process communication.

Basic example of serving your component is shown below. This example consists of a simple `JSON` plugin driver for the [Userprovider](https://github.com/cs3org/reva/blob/master/internal/grpc/services/userprovider/userprovider.go) service. You can find the example code [here](https://github.com/cs3org/reva/blob/master/examples/plugin/json/json.go).

```go

// main.go

import (
   	"github.com/cs3org/reva/v2/pkg/user"
    "github.com/hashicorp/go-plugin"
	revaPlugin "github.com/cs3org/reva/v2/pkg/plugin"
)

// Assume this implements the user.Manager interface
type Manager struct{}

func main() {
    // plugin.Serve serves the implementation over RPC to the core
	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig: revaPlugin.Handshake,
		Plugins: map[string]plugin.Plugin{
			"userprovider": &user.ProviderPlugin{Impl: &Manager{}},
		},
	})
}

```
The `plugin.Serve` method handles all the details of communicating with Reva core and serving your component over RPC. As long as your struct implements one of the exposed interfaces, Reva will be able to launch your plugin and use it. 

The `plugin.Serve` method takes in the plugin configuration, which you would have to define in your plugin source code:

- `HandshakeConfig`: The handshake is defined in `github.com/cs3org/reva/v2/pkg/plugin`

```go
var Handshake = plugin.HandshakeConfig{
	ProtocolVersion:  1,
	MagicCookieKey:   "BASIC_PLUGIN",
	MagicCookieValue: "reva",
}
```

- `Plugins`: Plugins is a map which maps the plugin implementation with the plugin name. Currently Reva supports 2 types of plugins (support for more to be added soon):
    - `userprovider`
    - `authprovider`

The implementation should be mapped to the above provided plugin names.



### Configuring and Loading Plugins

Runtime plugin can be configured using `.toml` files. We have ensured backwards compatibilty, hence configuring Reva to use runtime-plugins should feel as natural as configuring in-memory plugin drivers.

Reva provides 3 ways of loading plugins:

1. Providing path to already compiled go binary: If you have already compiled plugin binary, you just need to provide the path to the binary and rest of the work of loading the plugin would be taken care by the reva core. 

```
# Starting grpc userprovider service with json driver plugin
[grpc.service.userprovider]
driver = "/absolute/path/to/binary/json"

[grpc.service.userprovider.drivers.json]
user = "user.demo.json"
```

2. Provide path to the plugin source code: If you want the reva core to compile the plugins and then load the binary, you need to point to the go package consisting the plugin source code:

```
# Starting grpc userprovider service with json driver plugin
[grpc.service.userprovider]
driver = "/absolute/path/to/source/json"

[grpc.service.userprovider.drivers.json]
user = "user.demo.json"
```

3. Provide URL to plugin hosted on Github: If you provide Github URL, Reva would download the source code into a temporary directory, compile it into a binary and load that binary.

```
# Starting grpc userprovider service with json driver plugin
[grpc.service.userprovider]
driver = "https://github.com/jimil749/json"

[grpc.service.userprovider.drivers.json]
user = "user.demo.json"
```
