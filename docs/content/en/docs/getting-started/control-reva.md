---
title: "Control the Reva server"
linkTitle: "Control the Reva server"
weight: 4
description: >
  Control the Reva daemon to do hot reloads and configuration changes
---

The Reva daemon (revad) can be controlled with signals.
The process ID of the master process is written to the file specified by the **-p flag**.
If the file is not specified Reva will create a random pid file in the OS temporary folder.

```
  -p string
        pid file. If empty defaults to a random file in the OS temporary directory
```

The master process supports the following signals:

* **TERM, INT**: fast shutdown.
* **QUIT**: graceful shutdown.
* **HUP**: for configuration reloads.

## Changing Configuration

In order for revad to re-read the configuration file, a HUP signal should be sent to the master process.
The master process forks a new child that checks the configuration file for syntax validity, 
then tries to apply the new configuration, and inherits listening sockets.
If this fails, it kills itself and the parent process continues to work with the old configuration.

If this succeeds, the forked child sends a message to the old parent process requesting it to shut down gracefully.
The parent process closes the listening sockets and continue to service old clients.

After all clients are serviced, the old process is killed.

Let’s illustrate this by example. Imagine that revad is run on Darwin and the command:

```
ps axw -o pid,user,%cpu,command | egrep '(revad|PID)'
```

produces the following output:

```
PID   USER              %CPU COMMAND
46011  gonzalhu           0.0 revad -c /etc/revad/revad.toml -p /var/run/revad.pid
```

If HUP is sent to the master process, the output becomes:

```
PID   USER              %CPU COMMAND
46491   gonzalhu           0.0 revad -c /etc/revad/revad.toml -p /var/run/revad.pid
```

## Upgrading Executable on the Fly

In order to upgrade the server executable, the new executable file 
should be put in place of the old. After that, an HUP signal should be 
sent to the master process.
