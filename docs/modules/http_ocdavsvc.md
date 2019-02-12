# HTTP Service: ocdavsvc

This service exposes an [ownCloud](https://ownlcloud.org/)
WebDAV endpoint. This service allows ownCloud sync clients to connect
to this endpoint to synchronize files against the configured storage provider.
This service also allows to mount the storage provider using a WebDAV mount
from many operative systems, like Finder for MacOS, Network Drive for Windows or
davfs2 on Linux.

To enable the service:

```
[http]
enabled_services = ["ocdavsvc"]
```

Example configuration:

```
[http.services.ocdavsvc]
prefix = "webdav"
```

## Directives

```
Syntax:  prefix = string
Default: prefix = "webdav"
```

prefix specifies where the service should be exposed.
For example, if the prefix is "myservice", it will be
reachable at [http://localhost:9998/myservice](http://localhost:9998/myservice)


```
Syntax:  chunk_folder = string
Default: chunk_folder = "/tmp"
```

chunk_folder specifies where file chunks will be stored
on the local filesystem. The default is to use the 
system default for a temporary folder (```echo $TMPDIR```).

```
Syntax:  storageprovidersvc = string
Default: storageprovidersvc = 0.0.0.0:9999
```

storageprovidersvc specifies the location of the storage provider.
The ocdavsvc service acts as a protocol translator between WebDAV and 
the gRPC CS3 API for StorageProvider.
