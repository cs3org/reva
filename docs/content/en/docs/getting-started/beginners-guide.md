---
title: "Beginner's Guide"
linkTitle: "Beginner's Guide"
weight: 5
description: >
  Start playing with Reva and its services
---

This guide gives a basic introduction to Reva and describes some simple tasks that can be done with it.
This guide assumes that Reva is already installed on the reader's machine.
If this isn't the case refer to [Install Reva]({{< relref "docs/getting-started/install-reva" >}})

This guide describes how to start and stop the **Reva daemon (revad)**, and reload its configuration, explains the structure of the configuration
file and describes how to set up revad to serve some basic services.


## Creating a config

Reva stores its configuration in a toml file, in this example we store the config in a file named
*revad.toml*.

```
[shared]
jwt_secret = "mysecret"

[http.services.helloworld]
```

In this configuration we are telling Reva that for signing secrets we are going to use the secret "mysecret". Reva doesn't provide any default
secret and will not allow you to run if you don't define a secret. Default secrets can open severe security breaches and by forcing you to define your own we improve the security of your installation.

The line `[http.services.helloword]` enables a Hello World service.

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

For more information on sending signals to revad, see [Controlling Reva]({{< ref "docs/getting-started/control-reva" >}}).

## Running your first service

Reva configuration file is written in the [TOML](https://github.com/toml-lang/toml) language.

Reva is controlled by configuration directives that are divided into 4 sections:

* Core: for tweaking cpu and other parameters for the running process.
* Log: for configuration of the logging system.
* HTTP: for configuration of HTTP services and middlewares.
* GRPC: for configuration of gRPC services and interceptors.

An example configuration file is the following:

```
[shared]
jwt_secret = "mysecret"

[http.services.helloworld]
```

Running revad, will output some lines similar to these:

```
revad -c revad.toml
10:34AM INF Users/gonzalhu/Developer/reva/cmd/revad/runtime/runtime.go:72 > host info: red pid=59366
10:34AM INF Users/gonzalhu/Developer/reva/cmd/revad/runtime/runtime.go:148 > running on 4 cpus pid=59366
10:34AM INF Users/gonzalhu/Developer/reva/cmd/revad/internal/grace/grace.go:181 > pidfile saved at: /var/folders/72/r1bmgjg92p730hq9bpshxssr0000gn/T/revad-ae72db53-3954-4fea-bafb-2882dc4196c7.pid pid=59366 pkg=grace
10:34AM INF Users/gonzalhu/Developer/reva/pkg/rhttp/rhttp.go:203 > http service enabled: helloworld@/ pid=59366 pkg=rhttp
10:34AM INF Users/gonzalhu/Developer/reva/pkg/rhttp/rhttp.go:261 > unprotected URL: / pid=59366 pkg=rhttp
10:34AM INF Users/gonzalhu/Developer/reva/pkg/rhttp/rhttp.go:108 > http server listening at http://localhost:9998 pid=59366 pkg=rhttp
```

Let's analyze the output:

* The first line tells us some information about the host running the process.
* The second line tells us how many CPUs are used by the process.
* The  third line tells us where the pid file is stored.
* The fourth line tells us that an HTTP service has been enabled and where it is exposed, in this case the helloworld service is exposed at the root URL (/).
* The fifth line tells us that the URL starting with / is not protected by authentication, meaning is publicly accessible.
* The last line tells us where the HTTP server is listening.


**Congratulations, you've run your first Reva service!**

You can head to [Tutorials]({{< ref "docs/tutorials" >}}) to find more advanced guides.
