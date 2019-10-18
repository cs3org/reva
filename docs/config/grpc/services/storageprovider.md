# gRPC Service: storageprovider

To enable the service:

```
[grpc]
enabled_services = ["storageprovider"]
```

Example configuration:

```
[grpc.services.storageprovider]
driver = "local"
mount_path = "/localfs"
mount_id = "123e4567-e89b-12d3-a456-426655440000"
tmp_folder = "/var/tmp"

[grpc.services.storageprovider.drivers.local]
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

#### Directives

```
Syntax:  root = string
Default: root = "/tmp"
```

root specifies the directory in the local filesystem for storing data.


### EOS driver - [CERN Open Storage](http://eos.web.cern.ch/)
The EOS driver stores the files in a remote EOS storage system.


Example configuration: 

```
[grpc.services.storageprovidersvc.drivers.eos]
namespace = "/eos/user/"
master_url = "root://eosuser.example.org"
```

#### Directives

```
Syntax:  namespace = string
Default: namespace = "/eos"
```
namespace speficies the namespace on the remote EOS
storage system to perform storage operations.

```
Syntax:  eos_binary = string
Default: eos_binary = "/usr/bin/eos"
```
eos_binary specifies the location of the eos client binary.

```
Syntax:  xrdcopy_binary = string
Default: xrdcopy_binary = "/usr/bin/xrdcopy"
```

xrdcopy_binary specifies the location of the xrdcopy client binary.

```
Syntax:  master_url = string
Default: master_url = "root://example.org"
```
master_url specifies the master EOS MGM url.

```
Syntax:  slave_url = string
Default: slave_url = "root://example.org"
```
slave_url specifies the slave EOS MGM url.

```
Syntax:  cache_directory = string
Default: cache_directory = "/tmp"
```

cache_directory specifies where to store temporary files.
The default value is system default for a temporary folder (```echo $TMPDIR```).

```
Syntax:  show_hidden_sys_files = true | false
Default: show_hidden_sys_files = false
```

If show_hidden_sys_files is set to true, system files
used by EOS are exposed to the clients. System files follow the pattern
*.sys.** like version folders (.sys.v#.*) or atomic files (.sys.a#.).


```
Syntax:  force_single_user_mode = true | false
Default: force_single_user_mode = false
```

If force_single_user_mode is set all EOS command sent to the EOS
storage system will be sent as a single user specified by the *single_username*
directive. This directive is usefull when the access to EOS is done by 
web servers like Apache and all the commands run as www-data or apache unix user.

```
Syntax:  single_username = string
Default: single_username = ""
```

single_username specifies the unix account for run EOS commands. 


### Owncloud data directory
The `owncloud` driver stores the files in a local filesytem using the owncloud data directory layout. Files will be assigned a uuid, stored in extended attributes and cahced via redis.

Example configuration: 

```
[grpc.services.storageprovidersvc.drivers.owncloud]
datadirectory = "/data"
```

#### Directives

```
Syntax:  datadirectory = string
Default: datadirectory = ""
```

datadirectory specifies the directory in the local filesystem for storing data.

```
Syntax:  scan = boolean
Default: scan = true
```

scan will scan files on the first request to index file ids into the configured redis server

```
Syntax:  autocreate = boolean
Default: autocreate = true
```

autocreate will create the home dir and necessary subfolders for new users

```
Syntax:  redis = string
Default: redis = ":6379"
```

redis is used to store fileid to path mappings