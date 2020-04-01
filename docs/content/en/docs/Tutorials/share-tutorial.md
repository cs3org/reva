---
title: "Try out the share functionality in Reva"
linkTitle: "Share functionality"
weight: 5
description: >
  Try the share functionality in Reva locally.
---

This is a guide on how to try the share functionality in Reva in your local environment.

## Prerequisites
* golang
* make/automake
* git
* curl or wget

## 1. Clone the Reva repos
Clone the reva repo from https://github.com/cs3org/reva 

```
git clone https://github.com/cs3org/reva
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
make build-reva
```

```
make build-revad
```

## 4. Run Reva
Now you need to run Revad (the Reva deamon). Follow these steps
from the *reva* folder:

```
cd examples/ocmd/
``` 

```
../../cmd/revad/revad -dev-dir .
``` 

The Reva daemon (revad) should now be running.

## 5. Prepare an example file
Open a new terminal and go to your reva folder.

```
echo "Example file" > example.txt
```

## 6. Log in to reva
```
./cmd/reva/reva login basic
```

If you now get an error saying that you need to run reva configure, do as follows:
Run:

```
./cmd/reva/reva configure
```

and use 

```
host: 127.0.0.1:19000
```

Once configuered run:

```
./cmd/reva/reva login basic
```

And use the following log in credentials:

```
login: einstein
password: relativity
```

## 7. Upload the exmaple.txt file
Create container folder:

```
./cmd/reva/reva mkdir /einstein/
```

Upload the example file:

```
./cmd/reva/reva upload example.txt /einstein/example.txt
```

## 8. Share the file
Create share resource (with this, f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c, id you’re sharing with “marie”, you can find all users in examples/ocmd/users.demo.json) :
Use curl:

```
curl --request POST \
  --url 'http://127.0.0.1:19001/ocm/shares?path=example.txt&shareWith=f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c&=' \
  --header 'authorization: Basic ZWluc3RlaW46cmVsYXRpdml0eQ=='
```

Or use wget

```
wget --method POST \
  --header 'authorization: Basic ZWluc3RlaW46cmVsYXRpdml0eQ==' \
  --output-document \
  - 'http://127.0.0.1:19001/ocm/shares?path=example.txt&shareWith=f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c&='
```

## 9. List all shares resource
This will list all shared resources and give you some metadata
Use curl:

```
curl --request GET \
  --url http://127.0.0.1:19001/ocm/shares \
  --header 'authorization: Basic ZWluc3RlaW46cmVsYXRpdml0eQ=='
```  

Or use wget:

```
wget --quiet \
  --method GET \
  --header 'authorization: Basic ZWluc3RlaW46cmVsYXRpdml0eQ==' \
  --output-document \
  - http://127.0.0.1:19001/ocm/shares
```

## 10.Get one share resource
Use the share’s opaque_id you can see it in the response from “list all shares”
Use curl:

```
curl --request GET \
  --url http://127.0.0.1:19001/ocm/shares/*opaque_id* \
  --header 'authorization: Basic ZWluc3RlaW46cmVsYXRpdml0eQ=='
```

Or use wget:

```
wget --quiet \
  --method GET \
  --header 'authorization: Basic ZWluc3RlaW46cmVsYXRpdml0eQ==' \
  --output-document \
  - http://127.0.0.1:19001/ocm/shares/*opaque_id*
```
