---
title: "Use Phoenix with Reva"
linkTitle: "Use Phoenix with Reva"
weight: 5
description: >
  Connect Phoenix and Reva locally
---

This is a guide on how you can run both Phoenix and Reva locally in a dev environment. 
Phoenix is the new ownCloud frontend application and can be connected to Reva.

## Prerequisites
If you encounter strange problems, please check which version of the prerequisites you are running, it might be that you need to update/downgrade. For reference ask someone who already has reva and Phoenix running, they will have updated information on the versions.
* golang >= 1.12
* make/automake
* git >= 2
* node (check engines information in phoenix/package.json)
* yarn >= 1.0

## 1. Clone the Phoenix and Reva repos
Clone the phoenix repo from https://github.com/owncloud/phoenix 

```
git clone https://github.com/owncloud/phoenix
```

Clone the reva repo from https://github.com/cs3org/reva 

```
git clone https://github.com/cs3org/reva
```

## 2. Build Reva
Follow the instructions in https://reva.link/docs/getting-started/install-reva/ for how to build reva. If you will do local
changes in reva, follow the "Build from sources" instructions.

## 3. Set up Phoenix
Go to your Phoenix folder 

```
cd ../phoenix
```

Copy the *config.sample.json* file to *config.json* with the following command:

```
cp ../reva/examples/oc-phoenix/phoenix.oidc.config.json config.json
```


## 4. Run Reva
Now you need to run Revad (the Reva daemon). Follow these steps
from the *reva* folder:

```
cd examples/oc-phoenix/ && ../../cmd/revad/revad -dev-dir .
``` 

The Reva daemon (revad) should now be running.

## 5. Run Phoenix
Now you also need to run Phoenix, open another terminal tab and follow these steps from the *phoenix* folder.

Install all packages and build the project (this you only need to do once):

```
yarn install
```

```
yarn dist
```

Run Phoenix locally:

```
yarn watch-all
``` 

## 6. Open Phoenix 
You should now have both Reva and Phoenix up and running. You can access Phoenix on ```http://localhost:8300```.
Log in using username *einstein* and password *relativity*. 

## 7. Local changes
If you now do changes in Phoenix they will live changed on the opened tab *http://localhost:8300*.
You can check for example, change the name of one of the navItems in "default.js".

## 8. Enjoy your new Reva and Phoenix set up!
