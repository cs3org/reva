# Getting started with REVA

This guide assumes that you have REVA already installed in your system.

## Basic configuration

```
$ echo "" > ~/revad.toml
$ revad -c ~/revad.toml 
9:20AM INF dev/reva/cmd/revad/main.go:94 > version= commit= branch= go_version= build_date= build_platform= pid=2177
9:20AM INF dev/reva/cmd/revad/main.go:95 > running on 4 cpus pid=2177
9:20AM INF dev/reva/cmd/revad/main.go:118 > nothing to do, no grpc/http enabled_services declared in config pid=2177
```

An empty configuration will output those lines, stating that no services have been enabled and therefore the program exists.
Let's change the log output format to JSON.

```
$ cat ~/revad.toml
[log]
mode = "json"
```

```
$ revad -c ~/revad.toml 
{"level":"info","pid":9355,"time":"2019-10-16T09:22:17+02:00","caller":"/home/gonzalhu/dev/reva/cmd/revad/main.go:94","message":"version= commit= branch= go_version= build_date= build_platform="}
{"level":"info","pid":9355,"time":"2019-10-16T09:22:17+02:00","caller":"/home/gonzalhu/dev/reva/cmd/revad/main.go:95","message":"running on 4 cpus"}
{"level":"info","pid":9355,"time":"2019-10-16T09:22:17+02:00","caller":"/home/gonzalhu/dev/reva/cmd/revad/main.go:118","message":"nothing to do, no grpc/http enabled_services declared in config"}
```

REVA by default will use all the available CPU cores, let's change that to use only 50%:

```
[core]
max_cpus = "50%"
```

```
$ revad -c ~/revad.toml 
9:25AM INF dev/reva/cmd/revad/main.go:94 > version= commit= branch= go_version= build_date= build_platform= pid=19783
9:25AM INF dev/reva/cmd/revad/main.go:95 > running on 2 cpus pid=19783
9:25AM INF dev/reva/cmd/revad/main.go:118 > nothing to do, no grpc/http enabled_services declared in config pid=19783

```


## HTTP services

Let's enable the helloworld service:

```
$ cat revad.toml 
[http]
enabled_services = ["helloworldsvc"]
```

```
$ revad -c ~/revad.toml 
9:27AM INF dev/reva/cmd/revad/main.go:94 > version= commit= branch= go_version= build_date= build_platform= pid=24558
9:27AM INF dev/reva/cmd/revad/main.go:95 > running on 4 cpus pid=24558
9:27AM INF dev/reva/cmd/revad/grace/grace.go:181 > pidfile saved at: /tmp/gonzalhu/revad-ea51bb72-2d20-403e-a7db-73843e530115.pid pid=24558 pkg=grace
9:27AM INF dev/reva/cmd/revad/httpserver/httpserver.go:233 > http service enabled: helloworldsvc@/ pid=24558 pkg=httpserver
9:27AM INF dev/reva/cmd/revad/httpserver/httpserver.go:134 > http server listening at http://localhost:9998 pid=24558 pkg=httpserver
```

When the directive **enabled_services** is declared and the value is not empty, REVA will start an HTTP server with the declared services, in this case, the helloworld service. 

When REVA starts it will create a PID (process ID file), that will contain the PID of the REVA process, this file is needed to do reloads of the service as explained in [Controlling REVA](/controlling-reva.md). If no **-p flag** is specified, REVA will create the PID file in the operating system temporary directory, in this example: `/tmp/gonzalhu/revad-ea51bb72-2d20-403e-a7db-73843e530115.pid`.

The line *http service enabled: helloworldsvc@/* indicated that the helloworld service has been enabled and it can be reach at the root URL (/).

We can *curl* this endpoint:

```
$ curl http://localhost:9998
Hello World!
```

In the log lines of the server we see the result of the HTTP request:

```
9:34AM INF dev/reva/cmd/revad/svcs/httpsvcs/handlers/log/log.go:112 > http end="16/Oct/2019:09:34:11 +0200" host=127.0.0.1 method=GET pid=13968 pkg=httpserver proto=HTTP/1.1 size=12 start="16/Oct/2019:09:34:11 +0200" status=200 time_ns=67614 traceid=af56a393712698d362b0939b2caabb72 uri=/ url=/

```
