---
title: "Getting Started"
linkTitle: "Getting Started"
weight: 2
description: >
  What does your user need to know to try your project?
---

{{% pageinfo %}}
This is a placeholder page that shows you how to use this template site.
{{% /pageinfo %}}

Information in this section helps your user try your project themselves.

* What do your users need to do to start using your project? This could include downloading/installation instructions, including any prerequisites or system requirements.

* Introductory “Hello World” example, if appropriate. More complex tutorials should live in the Tutorials section.

Consider using the headings below for your getting started page. You can delete any that are not applicable to your project.

## Prerequisites

Are there any system requirements for using your project? What languages are supported (if any)? Do users need to already have any software or tools installed?

## Installation

Where can your user find your project code? How can they install it (binaries, installable package, build from source)? Are there multiple options/versions they can install and how should they choose the right one for them?

## Setup

Is there any initial setup users need to do after installation to try your project?

## Try it out!

Can your users test their installation, for example by running a commmand or deploying a Hello World example?


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
