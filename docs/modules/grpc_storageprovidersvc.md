# gRPC Service: storageprovidersvc

To enable the service:

```
[grpc]
enabled_services = ["storageprovidersvc"]
```

Example configuration:

```
[grpc.services.storageprovidersvc]
driver = "local"
mount_path = "/localfs"
mount_id = "123e4567-e89b-12d3-a456-426655440000"
tmp_folder = "/var/tmp"

[grpc.services.storageprovidersvc.drivers.local]
root = "/var/data"
```

## Directives

```
Syntax:  mount_path = string
Default: mount_path = ""
```
mount_path specifies where to mount the storage provider
in the global REVA namespace. This directive needs to be specified
to run the service.

```
Syntax:  mount_id = string
Default: mount_id = ""
```
mount_id specifies the identifier to append to internal object ids
so they become routable accross the REVA global namespace. This directive
needs to be specified to run the service.

```
Syntax:  tmp_folder = string
Default: tmp_folder = "/tmp"
```
tmp_folder specifies where temporary files will be stored
on the local filesystem. The default is to use the 
system default for a temporary folder (```echo $TMPDIR```).

```
Syntax:  driver = string
Default: driver = "local"
```

driver specifies the filesystem driver to use for the storage provider.
Available drivers shipped with REVA can be consulted at the end of this section.
The default is to use a local filesystem to store the files.
storageprovidersvc specifies the location of the storage provider.

## Storage drivers

### Local filesystem
The local driver stores the files in a local filesytem.

Example configuration: 

```
[grpc.services.storageprovidersvc.drivers.local]
root = "/var/data"
```

### EOS driver - [CERN Open Storage](http://eos.web.cern.ch/)
The EOS driver stores the files in a remote EOS storage system.


Example configuration: 

```
[grpc.services.storageprovidersvc.drivers.eos]
namespace = "/eos/user/"
master_url = "root://eosuser.example.org"
```
