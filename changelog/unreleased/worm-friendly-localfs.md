Enhancement:
Modifications in the localfs storage driver so that it can use a dCache nfs4.1 mount as file system

Modifications:
pkg/storage/utils/localfs/localfs.go and pkg/storage/fs/localhome have been modified so that DataDirectory and Shadow are now configurable.
pkg/storage/utils/localfs/uploads.go has been modified so that the uploaded file can be moved to a directory on a different file system. os.Rename cannot do that.

Configuration:
In the appropriate toml file "data_directory" and "shadow" can be defined in the [grpc.services.storageprovider.drivers.localhome] and [http.services.dataprovider.drivers.localhome] section.

example:
[grpc.services.storageprovider.drivers.localhome]
data_directory = "/dCache/data"
shadow = "/dCache/.shadow"

[http.services.dataprovider.drivers.localhome]
data_directory = "/dCache/data"
shadow = "/dCache/.shadow"

with a root as /var/tmp/reva, create a directory /var/tmp/reva/dCache and mount dCache on this directory. data_diretcory and shadow should be both on the dCache file system to make deleting and restoring files fast. The uploads diretcory should be on the local file system of the node since files are appended there and dCache cannot handle that.

https://github.com/cs3org/reva/pull/2455
