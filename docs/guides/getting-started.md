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
9:25AM INF dev/reva/cmd/revad/main.go:95 > **running** on 2 cpus pid=19783
9:25AM INF dev/reva/cmd/revad/main.go:118 > nothing to do, no grpc/http enabled_services declared in config pid=19783

```


