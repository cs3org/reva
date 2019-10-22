# gRPC functionality

Example configuration:

```
[grpc]
network = tcp
address = 0.0.0.0:9999
enabled_services = ["storageprovidersvc"]

[grpc.services.storageprovidersvc]
driver = "local"
mount_path = "/localfs"
mount_id = "123e4567-e89b-12d3-a456-426655440000"
tmp_folder = "/var/tmp"

[grpc.services.storageprovidersvc.drivers.local]
root = "/var/data"
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
Default: address = "0.0.0.0:9999"
```

address specifies the listening address for connections.

```
Syntax:  enabled_services = [string, string, ...]
Default: enabled_services = []
```
enabled_services specifies the grpc services exposed by revad.
The value is a list containing the names of the services.
By default, no service is exposed.

```
Syntax:  shutdown_deadline = int
Default: shutdown_deadline = 60
```
shutdown_deadline specifies how much time in seconds to wait for the 
grpc server to shutdown. Once the deadline is reached, ongoing requests that
did not finish will be aborted.
