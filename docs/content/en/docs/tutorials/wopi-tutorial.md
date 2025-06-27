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
* golang >= 1.24
* make/automake
* git >= 2
* python >=3.7

## 1. Clone the wopiserver and Reva repos
Clone the wopiserver repo:

```
git clone https://github.com/cs3org/wopiserver
```

Clone the reva and the configs repos:

```
git clone https://github.com/cs3org/reva
git clone https://github.com/cs3org/reva-configs
```

## 2. Configure Reva
Add `disable_tus = true` under `[http.services.dataprovider]` and under `[grpc.services.storageprovider]` in the file `server-1.toml`.

## 3. Build Reva
Follow the instructions in https://reva.link/docs/getting-started/install-reva/ for how to build reva. If you will do local
changes in reva, follow the "Build from sources" instructions.

## 4. Run Reva
Now you need to run Revad (the Reva daemon). Follow these steps
from the *reva* folder:

```
cd ~/reva-configs/ocm/
~/reva/cmd/revad/revad -c server-1.toml & ~/reva/cmd/revad/revad -c server-2.toml &
``` 

The Reva daemon (revad) should now be running.

## 5. Configure the wopiserver
Follow the instructions in the readme for running the server locally ("Run the WOPI server locally", https://github.com/cs3org/wopiserver).
You will need to do some changes in the config file, but you can start from this [reference configuration](https://github.com/cs3org/wopiserver/blob/master/docker/etc/wopiserver.cs3.conf).

## 6. Run wopiserver
Run according to instructions in the readme ("Run the WOPI server locally", https://github.com/cs3org/wopiserver).

## 7. Local changes
To try the connection you could for example go to a new reva terminal window and type 
`~/reva/cmd/reva/reva -insecure login basic` - use einstein and relativity as log in ccredentials
`~/reva/cmd/reva/reva -insecure open-in-app /home/example.txt read` - this should print out the app provider url in your terminal.

## 8. Enjoy your new Reva and wopiserver set up!
