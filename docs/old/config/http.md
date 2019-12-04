# HTTP functionality

Example configuration:

```
[http]
network = tcp
address = 0.0.0.0:9998
enabled_services = ["helloworldsvc"]

[http.services.helloworldsvc]
hello_message = "Ola Mundo!"
```

## Directives

```
Syntax:  network = tcp
Default: network = tcp
```
network specifies what network type to listen for requests. The only supported
network type for the moment is a tcp socket.

```
Syntax:  address  = string
Default: address = "0.0.0.0:9998"
```

address specifies the listening address for connections.

```
Syntax:  enabled_services = [string, string, ...]
Default: enabled_services = []
```
enabled_services specifies the http services exposed by revad.
The value is a list containing the names of the services.
By default, not service is exposed.

```
Syntax:  enabled_middlewares = [string, string, ...]
Default: enabled_middlewares = ["log", "trace"]
```
enabled_middlewares specifies the http middlwares used on the 
HTTP request->response chain.
The value is a list containing the names of the middlewares.
By default, the log and trace middlewares are enabled.
