# REVA Documentation

:warning: REVA is a very young project and documentation is subject to change very often!

* [Installing REVA](./installing-reva.md)
* [Building REVA from Sources](./building-reva.md)
* [Beginner's Guide](./beginner-guide.md)
* [Controlling REVA](./controlling-reva.md)
* [Command-line parameters](./command-line.md)

## Config reference

* [:link:](https://github.com/cs3org/reva/blob/master/cmd/revad/main.go#L368)  [Core](./modules/core.md) 
* [:link:](https://github.com/cs3org/reva/blob/master/cmd/revad/httpserver/httpserver.go#L74) [HTTP](./modules/http.md)
* [:link:](https://github.com/cs3org/reva/blob/master/cmd/revad/grpcserver/grpcserver.go#L86) [GRPC](./modules/grpc.md) 

>**HTTP services**

* [helloworldsvc](./modules/http_helloworldsvc.md)
* [prometheussvc](./modules/http_prometheussvc.md)
* [ocdavsvc](./modules/http_ocdavsvc.md)

>**HTTP middlewares**

* [auth](./modules/http_middleware_auth.md)
* [log](./modules/http_middleware_log.md)
* [trace](./modules/http_middleware_trace.md)

>**GRPC Services**

* [authsvc](./modules/grpc_authsvc.md)
* [storagebrokersvc](./modules/grpc_storagebrokersvc.md)
* [storageprovidersvc](./modules/grpc_storageprovidersvc.md)

>**GRPC interceptors**


