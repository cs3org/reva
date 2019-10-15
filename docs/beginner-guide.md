# Beginner's Guide

This guide gives a basic introduction to revad and describes some simple tasks that can be done with it.
This guide assumes that revad is already installed on the reader's machine.
If this is not, see [Installing REVA](./installing-reva.md).

This guide describes how to start and stop the **REVA daemon (revad)**, and reload its configuration, explains the structure of the configuration
file and describes how to set up revad to serve some basic services.

By default, the configuration file is named revad.toml and placed in the directory /etc/revad/revad.toml. 

## Starting, Stopping, and Reloading Configuration

To start revad, run the executable file:

```
revad -c revad.toml -p /var/tmp/revad.pid
```

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

*This command should be executed under the same user that started revad.*

Changes made in the configuration file will not be applied until the command to reload configuration is sent to revad or it is restarted. To reload configuration, execute: 

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

For more information on sending signals to revad, see [Controlling REVA](./controlling-reva.md).

## Configuration File’s Structure
revad configuration file is written in [TOML](https://github.com/toml-lang/toml) language.

revad consists of services which are controlled by directives specified in the configuration file.

An example configuration file is the following:

```
[http]
enabled_services = ["helloworldsvc"]
```

Running revad with this configuration directives will make revad listen on port 9998 for HTTP requests. Accessing *http://0.0.0.0:9998/* renders the message *Hello World!*.
The *helloworldsvc* is one of the many services available in revad. To modify the configuration for this service a new directive is added to the configuration:

```
[http]
enabled_services = ["helloworldsvc"]

[http.services.helloworldsvc]
message = "Ola Mundo!"
```

Reloading revad (```revad -s reload -p /var/tmp/revad.pid```) will render the new message.

