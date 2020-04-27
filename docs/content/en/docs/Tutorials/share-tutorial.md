
---
title: "OCM share functionality in Reva"
linkTitle: "OCM share functionality"
weight: 5
description: >
  OCM (Open Cloud Mesh) share functionality in Reva locally.
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
Go to the reva folder 

```
cd reva
```

We'll now build reva by running the following commands (you need to be in the *reva* folder)

```
make deps
```

```
make
```

## 4. Run Reva
Now we need to start two Reva deamons corresponding to two different mesh providers, thus enabling sharing of files between users belonging to these two providers. For our example,  we consider the example of CERNBox deployed at localhost:19001 and the CESNET owncloud at localhost:17001. Follow these steps:

```
cd examples/ocmd/
``` 

```
../../cmd/revad/revad -c ocmd-server-1.toml & ../../cmd/revad/revad -c ocmd-server-1.toml &
``` 

This should start two Reva daemon (revad) services at the aforementioned endpoints.

## 5. Invitation Workflow
Before we start sharing files, we need to invite users belonging to different mesh providers so that file sharing can be initiated.
### 5.1 Generate invite token
Generate an invite token for user einstein on CERNBox:
```
curl --location --request GET 'localhost:19001/ocm/invites/' \
--user einstein:relativity
```
You would get a response similar to
```
{"token":"2b51e7a3-7b19-482d-bbf6-b09e2375c0c2","user_id":{"idp":"http://cernbox.cern.ch","opaque_id":"4c510ada-c86b-4815-8820-42cdf82c3d51"},"expiration":{"seconds":1588069874}}
```
Each token is valid for 24 hours from the time of creation.
### 5.2 Accept the token
Now a user on a different mesh provider needs to accept this token in order to initiate file sharing. So we need to call the corresponding endpoint as user marie on CESNET.
```
curl --location --request POST \
'localhost:17001/ocm/invites/forward?token=2b51e7a3-7b19-482d-bbf6-b09e2375c0c2&providerDomain=http://cernbox.cern.ch' \
--user marie:radioactivity
```
An HTTP OK response indicates that the user marie has accepted an invite from einstein to receive shared files.

## 6. Sharing functionality
Creating shares at the origin is specific to each vendor and would have different implementations across providers. Thus, to skip the OCS HTTP implementation provided with reva, we would directly make calls to the exposed GRPC Gateway services through the reva CLI. 
### 6.1 Create a share on the original user's provider
#### 6.1.1 Create an example file
```
echo "Example file" > example.txt
```

#### 6.1.2 Log in to reva
```
./cmd/reva/reva login basic
```

If you now get an error saying that you need to run reva configure, do as follows:

```
./cmd/reva/reva configure
```

and use 

```
host: localhost:19000
```

Once configured, run:

```
./cmd/reva/reva login basic
```

And use the following log in credentials:

```
login: einstein
password: relativity
```
#### 6.1.3 Upload the example.txt file
Create container folder:

```
./cmd/reva/reva mkdir /home/
```

Upload the example file:

```
./cmd/reva/reva upload example.txt /home/example.txt
```
#### 6.1.4 Create the share
Call the ocm-share-create method with the required options. For now, we use the unique ID assigned to each user to identify the recipient of the share, but it can be easily modified to accept the email ID as well (`f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c` is the unique ID for the user marie; the list of all users can be found at `examples/ocmd/users.demo.json`).
```
./cmd/reva/reva ocm-share-create -grantee f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c -idp http://cesnet.cz /home/example.txt
```
This would create a local share on einstein's mesh provider and call the unprotected endpoint `/ocm/shares` on the recipient's provider to create a remote share. The response would look like:
```
+--------------------------------------+------------------------+--------------------------------------+----------------------------------------------------------------------------------------+-----------------------------------------------------------------------------------------------------------------+-------------------+------------------+--------------------------------------+--------------------------------+--------------------------------+
| #                                    | OWNER.IDP              | OWNER.OPAQUEID                       | RESOURCEID                                                                             | PERMISSIONS                                                                                                     | TYPE              | GRANTEE.IDP      | GRANTEE.OPAQUEID                     | CREATED                        | UPDATED                        |
+--------------------------------------+------------------------+--------------------------------------+----------------------------------------------------------------------------------------+-----------------------------------------------------------------------------------------------------------------+-------------------+------------------+--------------------------------------+--------------------------------+--------------------------------+
| c530f6b3-8eb7-4b68-af68-272ab8845bf8 | http://cernbox.cern.ch | 4c510ada-c86b-4815-8820-42cdf82c3d51 | storage_id:"123e4567-e89b-12d3-a456-426655440000" opaque_id:"fileid-home/example.txt"  | permissions:<get_path:true initiate_file_download:true list_container:true list_file_versions:true stat:true >  | GRANTEE_TYPE_USER | http://cesnet.cz | f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c | 2020-04-27 15:23:18 +0200 CEST | 2020-04-27 15:23:18 +0200 CEST |
+--------------------------------------+------------------------+--------------------------------------+----------------------------------------------------------------------------------------+-----------------------------------------------------------------------------------------------------------------+-------------------+------------------+--------------------------------------+--------------------------------+--------------------------------+
```
### 6.2 Accessing the share on the recipient's side
The recipient can access the list of shares shared with them. Similar to the create shares functionality, this implementation is specific to each vendor, so for the demo, we can access it through the reva CLI.

#### 6.2.1 Log in to reva
Reva CLI stores the configuration and authentication tokens in `.reva.config` and `.reva-token` files in the user's home directory. For now, this is not configurable so we need to set these again to access the platform as the user marie.
```
./cmd/reva/reva configure
```

and use 

```
host: localhost:17000
```

Once configured run:

```
./cmd/reva/reva login basic
```

And use the following log in credentials:

```
login: marie
password: radioactivity
```
#### 6.2.2 Access the list of received shares
Call the ocm-share-list-received method.
```
./cmd/reva/reva ocm-share-list-received
```
```
+--------------------------------------+------------------------+--------------------------------------+----------------------------------------------------------------------------------------+-------------------------------------------------------------------------------------------------------------------------------------------------------------------+-------------------+------------------+--------------------------------------+--------------------------------+--------------------------------+---------------------+
| #                                    | OWNER.IDP              | OWNER.OPAQUEID                       | RESOURCEID                                                                             | PERMISSIONS                                                                                                                                                       | TYPE              | GRANTEE.IDP      | GRANTEE.OPAQUEID                     | CREATED                        | UPDATED                        | STATE               |
+--------------------------------------+------------------------+--------------------------------------+----------------------------------------------------------------------------------------+-------------------------------------------------------------------------------------------------------------------------------------------------------------------+-------------------+------------------+--------------------------------------+--------------------------------+--------------------------------+---------------------+
| e327bf7d-cda7-4cdc-bb82-fbeef017dd16 | http://cernbox.cern.ch | 4c510ada-c86b-4815-8820-42cdf82c3d51 | storage_id:"123e4567-e89b-12d3-a456-426655440000" opaque_id:"fileid-home/example.txt"  | permissions:<get_path:true get_quota:true initiate_file_download:true list_grants:true list_container:true list_file_versions:true list_recycle:true stat:true >  | GRANTEE_TYPE_USER | http://cesnet.cz | f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c | 2020-04-27 15:23:18 +0200 CEST | 2020-04-27 15:23:18 +0200 CEST | SHARE_STATE_PENDING |
+--------------------------------------+------------------------+--------------------------------------+----------------------------------------------------------------------------------------+-------------------------------------------------------------------------------------------------------------------------------------------------------------------+-------------------+------------------+--------------------------------------+--------------------------------+--------------------------------+---------------------+
```

