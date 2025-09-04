# CephMount Storage Driver

CephMount provides a CephFS-compatible filesystem interface for Reva using fstab-based configuration.

## Requirements

### 1. Ceph Development Libraries

Install Ceph development libraries on your system:

```bash
# Fedora 42
sudo dnf install libcephfs-devel librados-devel

# CentOS/RHEL
sudo yum install libcephfs-devel librados-devel
```

### 2. CephFS Kernel Mount

You need a valid persistent CephFS kernel mount accessible by the user running Reva (typically root).
Refer to [Ceph fstab configuration](https://docs.ceph.com/en/reef/cephfs/mount-using-kernel-driver/#persistent-mounts).

```bash
ceph-mon.example.com:6789:/volumes/shared /mnt/cephfs ceph defaults,name=admin,secretfile=/etc/ceph/ceph.client.admin.key,conf=/etc/ceph/ceph.conf 0 2
```

This entry must be configured in your Reva config as the `fstabentry` parameter.

## Quick Start

### Configuration

Configure in your `config.toml`:

```toml
[grpc.services.storageprovider]
driver = "cephmount"

[grpc.services.storageprovider.drivers.cephmount]
fstabentry = "ceph-mon.example.com:6789:/volumes/shared /mnt/cephfs ceph defaults,name=admin,secretfile=/etc/ceph/ceph.client.admin.key,conf=/etc/ceph/ceph.conf 0 2"
root = "/"

[http.services.dataprovider]
driver = "cephmount"

[http.services.dataprovider.drivers.cephmount]
fstabentry = "ceph-mon.example.com:6789:/volumes/shared /mnt/cephfs ceph defaults,name=admin,secretfile=/etc/ceph/ceph.client.admin.key,conf=/etc/ceph/ceph.conf 0 2"
root = "/"
```

This example will convert Reva paths like `/projects/alabasta/planning.md`, to `/mnt/cephfs/projects/alabasta/planning.md` (when using localfs functions) and to `/volumes/shared/projects/alabasta/planning.md` when connecting directly to MDS. 

## Testing
There are 2 major testing scenarios:

```bash
# Unit tests (no CephFS required)
go test ./pkg/storage/fs/cephmount -v

# Integration tests (requires valid CephFS mount)
```bash
export CEPHMOUNT_FSTAB_ENTRY="your-fstab-entry-here"
go test -tags ceph ./pkg/storage/fs/cephmount -v
```

Depending if you run the tests as root or as another user (typically in your dev machines), some tests
like the privilege verification ones may be skipped due to lack of permsissions to switch uid/guid for the 
forked processes.

# Benchmarks

```
bash
go test ./pkg/storage/fs/cephmount -bench "Benchmark.*" -v
go test -tags ceph ./pkg/storage/fs/cephmount -bench "BenchmarkCeph.*" -v
```

## Development Mode

For testing without CephFS:

```toml
[grpc.services.storageprovider.drivers.cephmount]
testing_allow_local_mode = true  # Testing only!
root = "/tmp/testing"
```

**Never use `testing_allow_local_mode` in production!**

**Fucntions that require conversion between file ID (inode) and and path will always require 
access to the MDS  and therefore having a valid Ceph configuration. The tests handle this transparently, skipping any tests necessary**.

