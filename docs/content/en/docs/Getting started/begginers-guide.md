---
title: "Begginer's Guide"
linkTitle: "Begginer's Guide"
weight: 5
description: >
  Start playing with Reva and its services
---

This guide gives a basic introduction to Reva and describes some simple tasks that can be done with it.
This guide assumes that Reva is already installed on the reader's machine.
If this isn't the case refer to [Install Reva]({{< relref "docs/Getting Started/install-reva" >}})

This guide describes how to start and stop the **Reva daemon (revad)**, and reload its configuration, explains the structure of the configuration
file and describes how to set up revad to serve some basic services.


## Creating a config

Reva stores its configuration in a toml file, in this example we store the config in a file named
*revad.toml*.

```
[http]
enabled_services = ["helloworld"]
```

## Reloading the config

To start revad, run the executable file:

```
revad -c revad.toml -p /var/tmp/revad.pid
```

{{% alert title="The pid flag" color="warning" %}}
If you don't specify the pid file with the -p flag Reva will create one in the OS temporary directory.
You can always obtain the location of the file from the logs
{{% /alert %}}

Once revad is started, it can be controlled by invoking the executable with the -s parameter. Use the following syntax: 

```
revad -s <signal> -p /var/tmp/revad.pid
```

Where signal may be one of the following:

* stop — fast shutdown (aborts in-flight requests)
* quit — graceful shutdown
* reload — reloading the configuration file (forks a new process)

 For example, to stop revad gracefully, the following command can be executed: 

```
revad -s quit -p /var/tmp/revad.pid
```

{{% alert color="warning" %}}
This command should be executed under the same user that started the server.
{{% /alert %}}

Changes made in the configuration file will not be applied until the command to reload configuration is sent to revad. Let's reload the config: 

```
revad -s reload -p /var/tmp/revad.pid
```

Once the main process receives the signal to reload configuration, it checks the syntax validity of the new configuration file and tries to apply the configuration provided in it. If this is a success, the main process forks a new process. The new forked process will gracefully kill the parent process. During a period of time until all ongoing requests are served, both processes will share the same network socket, the old parent process will serve ongoing requests and the new process will serve only new requests. No requests are dropped during the reload. If the provided configuration is invalid, the forked process will die and the master process will continue serving requests.

A signal may also be sent to the revad process with the help of Unix tools such as the *kill* utility. In this case a signal is sent directly to a process with a given process ID. The process ID of the revad master process is written to the pid file, as configured with the *-s* flag. For example, if the master process ID is 1610, to send the QUIT signal resulting in revad’s graceful shutdown, execute: 

```
kill -s QUIT 1610
```

For getting the list of all running revad processes, the *ps* utility may be used, for example, in the following way: 

```
ps -ax | grep revad
```

For more information on sending signals to revad, see [Controlling Reva]({{< ref "docs/Getting Started/control-reva" >}}).

## Running your first service

Reva configuration file is written in the [TOML](https://github.com/toml-lang/toml) language.

Reva is controlled by configuration directives that are divided into 4 sections:

* Core: for tweaking cpu and other parameters for the running process.
* Log: for configuration of the logging system.
* HTTP: for configuration of HTTP services and middlewares.
* GRPC: for configuration of gRPC services and interceptors.

An example configuration file is the following:

```
[http]
enabled_services = ["helloworldsvc"]
```

Running revad, will output some lines similar to these:

```
$ revad -c /etc/revad/revad.toml 

1:22PM INF cmd/revad/main.go:94 > version=9cc0106 commit=dirty-9cc0106 branch=hugo-docs go_version=go1.12.9 build_date=2019-10-21T13:20:32+0200 build_platform=linux/amd64 pid=15369
1:22PM INF cmd/revad/main.go:95 > running on 4 cpus pid=15369
1:22PM INF cmd/revad/internal/grace/grace.go:181 > pidfile saved at: /tmp/revad-a23283b8-1e95-4bc1-987b-6e85452d0214.pid pid=15369 pkg=grace
1:22PM INF pkg/rhttp/rhttp.go:232 > http service enabled: helloworld@/ pid=15369 pkg=rhttp
1:22PM INF pkg/rhttp/rhttp.go:133 > http server listening at http://localhost:9998 pid=15369 pkg=rhttp
```

Let's analyze the output:

* The first line tells us what version of Reva we run, in this case, a development version.
* The second line tells us hown many CPUs are used by the process.
* The  third line tells us where the pid file is stored.
* The fourth line tells us  what HTTP service has been enabled and where is exposed, in this case the helloworld service is exposed at the root URL (/).
* The last line tells us where the HTTP server is listening.


**Congratulations, you've run your first Reva service!**

You can head to [Tutorials]({{< ref "docs/Tutorials" >}}) to find more advanced guides.

