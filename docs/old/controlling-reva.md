# Controlling REVA

revad can be controlled with signals. The process ID of the master process is written to the file */var/run/revad.pid* by default. This name may be changed with the  *-p* flag:

```
-p string
 	pid file (default "/var/run/revad.pid")
```

The master process supports the following signals:

* TERM, INT: fast shutdown
* QUIT: graceful shutdown
* HUP: changing configuration, starting new process with the new configuration, graceful shutdown of old parent processes

## Changing Configuration

In order for revad to re-read the configuration file, a HUP signal should be sent to the master process.
The master process forks a new child that checks the configuration file for syntax validity, 
then tries to apply new configuration, and inherits listening sockets.
If this fails, it kills itself and the parent process continues to work with old configuration.
If this succeeds, the forked child sends a message to old parent process requesting it to shut down gracefully.
Parent process close listening sockets and continue to service old clients.
After all clients are serviced, old process is shut down.

Let’s illustrate this by example. Imagine that revad is run on Darwin and the command:

```
ps axw -o pid,user,%cpu,command | egrep '(revad|PID)'
```

produces the following output:

```
PID   USER              %CPU COMMAND
46011  gonzalhu           0.0 ./revad -c revad.toml -p revad.pid
```

If HUP is sent to the master process, the output becomes:

```
PID   USER              %CPU COMMAND
46491   gonzalhu           0.0 ./revad -c revad.toml -p revad.pid
```

## Upgrading Executable on the Fly

In order to upgrade the server executable, the new executable file 
should be put in place of an old file first. After that, an HUP signal should be 
sent to the master process.
 The master process run the new executable file that in turn starts a new child process.
