---
title: "Run and develop the accept invite Phoenix app"
linkTitle: "Run and develop the accept invite Phoenix app"
weight: 5
description: >
  Run and develop the accept invite Phoenix app locally together with Phoenix and Reva
---

This is a guide on how you can run the accept invite app together with Pheonix and Reva locally in a dev environment. If you already have Reva and Phoenix you don't need to clone them, but you will need to congigure Phoenix (step 4) and have Reva and Phoenix running.

## 1. Clone the Phoenix and Reva repos
Clone the phoenix repo from https://github.com/owncloud/phoenix 

```
git clone https://github.com/owncloud/phoenix
```

Clone the reva repo from https://github.com/cs3org/reva 

```
git clone https://github.com/cs3org/reva
```

Clone the accept invite repo from https://github.com/sciencemesh/accept-invite

```
git clone git@github.com:sciencemesh/accept-invite.git
```

## 2. Build Reva
Go to your Reva folder 

```
cd reva
```

Now you need to build Reva by running the following commands (you need to be in the *reva* folder)

```
make deps
```

```
make
```

## 3. Set up Phoenix
Go to your Phoenix folder 

```
cd ../phoenix
```

Copy the *config.sample.json* file to *config.json* with the following command:

```
cp ../reva/examples/oc-phoenix/phoenix.oidc.config.json config.json
```

## 4. Configure Phoenix
In the config.json file in the Phoenix repo add:

```
  "external_apps": [
    {
      "id": "hello",
      "path": "http://localhost:10001/bundle.js"
    }
  ]
```

## 5. Run Reva
Now you need to run Revad (the Reva deamon). Follow these steps
from the *reva* folder:

```
cd examples/oc-phoenix/ && ../../cmd/revad/revad -dev-dir .
``` 

The Reva daemon (revad) should now be running.

## 6. Run Phoenix
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

## 7. Run accept invite
Lastly you need to run the application. Open yet another terminal tab and follow these steps.

```
yarn install
```

```
yarn watch
```

## 8. Open Phoenix and the accept invite application
You should now have both Reva and Phoenix up and running. You can access Phoenix on ```http://localhost:8300```. Log in using username *einstein* and password *relativity*. To go to the accept invite app, click the sqaures in the upper right corner and then "Accept invite".

## 9. Local changes
If you now do changes in the accept invite app they will live changed on the opened tab *http://localhost:8300*.
You can check for example, change some text in "accept-invite/ui/components/AcceptInvite.vue".

## 10. Enjoy your new accept invite application!
