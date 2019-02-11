# Beginner's Guide

This guide gives a basic introduction to revad and describes some simple tasks that can be done with it.
This guide assumes that revad is already installed on the reader's machine.
If this is not, see [Installing REVA](./installing-reva.md).

This guide describes how to start and stop the **REVA daemon (revad)**, and reload its configuration, explains the structure of the configuration
file and describes how to set up revad to serve static content.

By default, the configuration file is named revad.toml and placed in the directory /etc/revad/revad.toml. 

## Starting, Stopping, and Reloading Configuration

To start revad, run the executable file:

```
revad -c revad.toml -p revad.pid
```

Once revad is started, it can be controlled by invoking the executable with the -s parameter. Use the following syntax: 

```
revad -s signal
```

Where signal may be one of the following:

* stop — fast shutdown (aborts in-flight requests)
* quit — graceful shutdown
* reload — reloading the configuration file (forks a new process)

 For example, to stop revad gracefully, the following command can be executed: 

```
revad -s quit
```

*This command should be executed under the same user that started nginx.*
