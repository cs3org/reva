---
title: "Use WOPI with Reva"
linkTitle: "Use WOPI with Reva"
weight: 5
description: >
  Connect the cs3 wopiserver with Reva
---

This is a guide on how you can run both Reva and wopiserver locally in a dev environment. 
The wopiserver will allow you to connect Reva to online editors such as collabora.

## Prerequisites
If you encounter strange problems, please check which version of the prerequisites you are running, it might be that you need to update/downgrade. For reference ask someone who already has reva and Phoenix running, they will have updated information on the versions.
* golang >= 1.12
* make/automake
* git >= 2
* python >=3.7

## 1. Clone the wopiserver and Reva repos
Clone the wopiserver repo from https://github.com/cs3org/wopiserver

```
git clone https://github.com/cs3org/wopiserver
```

Clone the reva repo from https://github.com/cs3org/reva 

```
git clone https://github.com/cs3org/reva
```

## 2. Configure Reva
Add `disable_tus = true` under `[http.services.dataprovider]` and under `[grpc.services.storageprovider]` in the file `ocmd-server-1.toml`.

## 3. Build Reva
Follow the instructions in https://reva.link/docs/getting-started/install-reva/ for how to build reva. If you will do local
changes in reva, follow the "Build from sources" instructions.

## 4. Run Reva
Now you need to run Revad (the Reva daemon). Follow these steps
from the *reva* folder:

```
cd examples/ocm/ && ../../cmd/revad/revad -c ocmd-server-1.toml & ../../cmd/revad/revad -c ocmd-server-2.toml &.
``` 

The Reva daemon (revad) should now be running.

## 5. Configure the wopiserver
Follow the instructions in the readme for running the server locally ("Run the WOPI server locally", https://github.com/cs3org/wopiserver). You will need to do come changes in the config file, here is a more relevant example of a wopi config when runnning the server together with reva:

```
#
# wopiserver.conf - basic working configuration for a docker image
#

[general]
storagetype = cs3
port = 8880
allowedclients = localhost
oosurl = https://oos.web.cern.ch
codeurl = https://collabora.cern.ch:9980/byoa/collabora
codimdurl = http://cbox-wopidev-01.cern.ch:8000
tokenvalidity = 86400

wopiurl = localhost:8880
downloadurl = localhost:8880/wopi/cbox/download

# Logging level. Debug enables the Flask debug mode as well.
# Valid values are: Debug, Info, Warning, Error.
loglevel = Debug

[security]
usehttps = no

# location of the secret files. Requires a restart of the
# WOPI server when either the files or their content change.
wopisecretfile = /etc/wopi/wopisecret
iopsecretfile = /etc/wopi/iopsecret

[cs3]
revahost = localhost:19000
authtokenvalidity = 3600

[io]
# Size used for buffered xroot reads [bytes]
chunksize = 4194304 
```

## 6. Run wopiserver
Run according to instructions in the readme ("Run the WOPI server locally", https://github.com/cs3org/wopiserver).

## 7. Local changes
To try the connection you could for example go to a new reva terminal window and type 
`./cmd/reva/reva -insecure login basic` - use einstein and relativity as log in ccredentials
`./cmd/reva/reva -insecure open-file-in-app-provider /home/example.txt read` - this should print out the app provider url in your terminal. 

## 8. Enjoy your new Reva and wopiserver set up!
