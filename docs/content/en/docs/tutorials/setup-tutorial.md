---
title: "Setting up Reva and basic functionalities"
linkTitle: "Setting up Reva"
weight: 10
description: >
  Setting up and executing fundamental features
---

This is a guide on how to set up Reva in your local environment and try out some of its fundamental sync-and-share functionalities.

## 1. Architecture Overview

- Reva provides the options of running multiple microservices, which interact with each other over standard transport protocols to enable the functionalities of a sync-and-share system. Most of the microservices are built using the architecture described in the figure below.
- Reva supports running multiple drivers of the same service simultaneously. For example, it can support basic and OAuth2 authentication mechanisms, connected to file systems hosted locally or which are accessed over specified protocols such as xroot or NFS.
- The clients can access Reva through HTTP or GRPC endpoints. These requests are forwarded to the gateway, which, depending on the request paramaters, decides which driver or instance of the microservice it needs to be redirected to.
- These registries can be configured according to specified rules which can be used to implement sharding, load-balancing, and replication.

```
 client ----------> HTTPService ----------> GRPCGateway ------------> ServiceRegistry
    |      REST                    GRPC      |    |         GRPC
    |                                        |    |
    |                                        |    |
    +----------------------------------------+    +-----------------> ServiceDriver-A
                      GRPC                        |         GRPC
                                                  |
                                                  |
                                                  +-----------------> ServiceDriver-B
                                                            GRPC
 ```

## 2. Build Reva
Follow the instructions in https://reva.link/docs/getting-started/install-reva/ for how to build Reva. If you're making local changes, you'll need to build the binary again.

## 3. Start Reva Daemons
Reva daemons are spawned using configuration specified in `toml` format. Multiple microservices can run on the same server. For a minimal example with the default values, take a look at the [storage-references](https://github.com/cs3org/reva/tree/master/examples/storage-references) example in Reva. The `gateway.toml` starts these service at port 19000 while the storage providers run on ports 17000 and 18000. The `storageregistry` rules specify that the home directory requests for the users should go to the `storage-home` server while others (shares, project spaces) should be handled by a global storage provider. This can be visualised below.

```
          +                             +----+
          |     +--------H--------+     |    |
          |     |                 |     |    |
          +---->G /reva           M---->+    |
          |     |                 |     |    |
          |     +------local------+     |    |
          |                             |    |
          |                             |    |
          |     +--------H--------+     |    |
          |     |                 |     |    |
          +---->G /home           M---->+    |
          |     |                 |     |    |
          |     +----localhome----+     |    |
          +                             +----+

       GATEWAY    STORAGE PROVIDERS     SHARED
                                        VOLUME

    M: Mountpoint             H: HTTP exposed service
                              G: gRPC exposed service
```

You can start these three daemons using the `-dev-dir` flag which fires up a daemon for each toml file in a directory. The user, group and mesh provider services use JSON files by default as their data store, and expects these to be located at `/etc/revad`. Some of these commands may require `sudo`, depending on your system setup.

```
> mkdir -p /var/tmp/reva
> mkdir -p /etc/revad
> cp examples/storage-references/users.demo.json /etc/revad/users.json
> cp examples/storage-references/groups.demo.json /etc/revad/groups.json
> cp examples/storage-references/providers.demo.json /etc/revad/ocm-providers.json
> cmd/revad/revad -dev-dir examples/storage-references
```

## 4. Access through the CLI
One of the simplest clients is an interactive CLI tool, which connects to the gateway to provide access to the fundamental functionalities.
```
> cmd/reva/reva -insecure -host localhost:19000
reva-cli v1.6.0-25-g117adad6 (rev-117adad6)
Please use `exit` or `Ctrl-D` to exit this program.
```

The command `help` can be used to get details of all the available commands. You can login as one of the sample users then manipulate resources in users' home directories.
```
>> login basic
username: einstein
password: relativity
OK
>> ls /home
MyShares
>> mkdir /home/MyFolder
>> upload ~/a.txt /home/MyFolder/MyFile.txt
Local file size: 15 bytes
Data server: http://localhost:19001/datagateway
Allowed checksums: [type:RESOURCE_CHECKSUM_TYPE_MD5 priority:100  type:RESOURCE_CHECKSUM_TYPE_UNSET priority:1000 ]
Checksum selected: RESOURCE_CHECKSUM_TYPE_MD5
Local XS: RESOURCE_CHECKSUM_TYPE_MD5:085f396b2bdea443f3d5b889f84d49f5
File uploaded: 123e4567-e89b-12d3-a456-426655440000:fileid-einstein%2FMyFolder%2FMyFile.txt 15 /home/MyFolder/MyFile.txt

>> ls /home
MyFolder
MyShares

>> share-create -grantee f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c /home/MyFolder
+--------------------------------------+-----------------+--------------------------------------+-------------------------------------------------------------------------------------------+-----------------------------------------------------------------------------------------------------------------+-------------------+-----------------+--------------------------------------+-------------------------------+-------------------------------+
| #                                    | OWNER.IDP       | OWNER.OPAQUEID                       | RESOURCEID                                                                                | PERMISSIONS                                                                                                     | TYPE              | GRANTEE.IDP     | GRANTEE.OPAQUEID                     | CREATED                       | UPDATED                       |
+--------------------------------------+-----------------+--------------------------------------+-------------------------------------------------------------------------------------------+-----------------------------------------------------------------------------------------------------------------+-------------------+-----------------+--------------------------------------+-------------------------------+-------------------------------+
| d6e600f1-e9e3-40b4-bb13-f3848d904422 | localhost:20080 | 4c510ada-c86b-4815-8820-42cdf82c3d51 | storage_id:"123e4567-e89b-12d3-a456-426655440000" opaque_id:"fileid-einstein%2FMyFolder"  | permissions:<get_path:true initiate_file_download:true list_container:true list_file_versions:true stat:true >  | GRANTEE_TYPE_USER | localhost:20080 | f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c | 2021-03-10 18:02:08 +0100 CET | 2021-03-10 18:02:08 +0100 CET |
+--------------------------------------+-----------------+--------------------------------------+-------------------------------------------------------------------------------------------+-----------------------------------------------------------------------------------------------------------------+-------------------+-----------------+--------------------------------------+-------------------------------+-------------------------------+
```

Now the shared folder can be accessed on the recipient side.
```
>> login basic
username: marie
password: radioactivity
OK
>> share-list-received
+--------------------------------------+-----------------+--------------------------------------+-------------------------------------------------------------------------------------------+-----------------------------------------------------------------------------------------------------------------+-------------------+-----------------+--------------------------------------+-------------------------------+-------------------------------+---------------------+
| #                                    | OWNER.IDP       | OWNER.OPAQUEID                       | RESOURCEID                                                                                | PERMISSIONS                                                                                                     | TYPE              | GRANTEE.IDP     | GRANTEE.OPAQUEID                     | CREATED                       | UPDATED                       | STATE               |
+--------------------------------------+-----------------+--------------------------------------+-------------------------------------------------------------------------------------------+-----------------------------------------------------------------------------------------------------------------+-------------------+-----------------+--------------------------------------+-------------------------------+-------------------------------+---------------------+
| d6e600f1-e9e3-40b4-bb13-f3848d904422 | localhost:20080 | 4c510ada-c86b-4815-8820-42cdf82c3d51 | storage_id:"123e4567-e89b-12d3-a456-426655440000" opaque_id:"fileid-einstein%2FMyFolder"  | permissions:<get_path:true initiate_file_download:true list_container:true list_file_versions:true stat:true >  | GRANTEE_TYPE_USER | localhost:20080 | f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c | 2021-03-10 18:02:08 +0100 CET | 2021-03-10 18:02:08 +0100 CET | SHARE_STATE_PENDING |
+--------------------------------------+-----------------+--------------------------------------+-------------------------------------------------------------------------------------------+-----------------------------------------------------------------------------------------------------------------+-------------------+-----------------+--------------------------------------+-------------------------------+-------------------------------+---------------------+
>> share-update-received -state accepted d6e600f1-e9e3-40b4-bb13-f3848d904422
OK
>> ls /home/MyShares
MyFolder
```
