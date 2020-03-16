---
title: "Use Phoenix with Reva"
linkTitle: "Use Phoenix with Reva"
weight: 5
description: >
  Connect Phoenix and Reva locally
---

This is a guide on how you can run both Pheonix and Reva locally in a dev environment. Phoenix is a frontend application connected to an open cloud backend server through the Reva platform.

## 1. Clone the Phoenix and Reva repos

Clone the phoenix repo from https://github.com/owncloud/phoenix 

```git clone https://github.com/owncloud/phoenix```

Clone the reva repo from https://github.com/cs3org/reva 

```git clone https://github.com/cs3org/reva```

## 2. Build Reva
Go to your Reva folder 

```cd reva```

Now you need to build Reva by running the following commands (you need to be in the reva folder)

```make deps```

```make```

## 3. Set up Phoenix
Go to your Phoenix folder 

```cd ../phoenix```

Copy the config.sample.json file to config.json with the following command:

```cp ../reva/examples/oc-phoenix/phoenix.oidc.config.json config.json```


## 4. Run Phoenix and Reva
Nu you need to run Phoenix and Revad (the Reva deamon). This is done with the following steps:
In the Reva folder

```cd examples/oc-phoenix/ && ../../cmd/revad/revad -dev-dir .``` 

The Revad should now be running.
In the Phoenix folder (open another terminal tab):

```yarn watch-all``` 

## 5. Open Phoenix 
You should now have both Reva and Phoenix up. You can open Phoenix on localhost:8300.
Log in use username einstein and password relativity. 

## 6. Local changes
If you now do changes in phoenix they will directly appear on localhost:8300. You can chekc this by e.g. change the name of one of the navItems in "default.js".

## 7. Enjoy your new Reva and Pheonix set up!