---
title: "Setting up Reva with CephFS"
linkTitle: "Setting up Reva with CephFS"
weight: 10
description: >
  Setting up Reva with a CephFS cluster
---

This is a guide on how to set up Reva in your local environment and connect it to an existing CephFS cluster.

For questions on this tutorial plase refer to https://github.com/cs3org/reva/discussions/4610

### 1. CephFS setup
You need to have an existing CephFS installation in the machine where you will deploy Reva.
Even though is not needed for Reva to have CephFS mounted on the machine where Reva will run, we highly recommend it 
as it will make grasping the concepts much easier.

For this tutorial, we have a Ceph mount exposed under the mountpoint `/cephfs`.

```
$ cat /etc/fstab  | grep cephfs
cernbox@.cernbox=/                        /cephfs                 ceph    rbytes
```

```
$ df -h | grep ceph
10.81.22.151:6789,10.81.22.161:6789,10.81.22.171:6789:/  1.3P  650G  1.2P   1% /cephfs
```

The ceph configuration lives under `/etc/ceph`.

```
$ tree /etc/ceph/
/etc/ceph/
├── ceph.client.cernbox.keyring
├── ceph.conf
└── rbdmap
```

Your cluster details will differ, this is just an example configuration file.
```
$ cat /etc/ceph/ceph.conf
[global]
auth_client_required=cephx
fsid=f5195e24-158c-11ee-b338-5ced8c61b074
mon_host=[v2:10.81.22.151:3300/0,v1:10.81.22.151:6789/0],[v2:10.81.22.161:3300/0,v1:10.81.22.161:6789/0],[v2:10.81.22.171:3300/0,v1:10.81.22.171:6789/0]
```

```
cat /etc/ceph/ceph.client.cernbox.keyring
[client.cernbox]
key = mycephsecretkey==
```

With this information we can start setting up Reva.



## Reva setup


Follow the steps here:
https://reva.link/docs/getting-started/build-reva/

We also need the libcephfs library, depending on your OS the command to install will change, here is how you install it for Fedora 39:
```
dnf install libcephfs* -y
```

At this step you shoudl have a local clone of the Reva software:

```
git clone https://github.com/cs3org/reva
cd reva
make revad-ceph
make reva
./cmd/revad/revad -v
```

You can copy the binaries (`reva` is the client cli and `revad` is the daemon) to a default location so is available in your PATH:
```
cp ./cmd/revad/revad /usr/local/bin/revad
cp ./cmd/reva/reva /usr/local/bin/reva
```


### Creating test users
CephFS relies on the UNIX uid and guid attributes to perform access control.
For this example, we'll create `einstein` user with `uid=4000`:

```
$ sudo useradd -u 4000  einstein
$ id einstein
uid=4000(einstein) gid=4000(einstein) groups=4000(einstein)
```
### Create configuration files

For this tutorial, we'll use two files:
- `revad.toml` (main configuration file to run reva, preconfigured for Ceph cluster)
- `test_users.json` (configuration used to store users, only `einstein` is configured)

These files are available at https://github.com/cs3org/reva/tree/master/examples/cephfs

Copy the `revad.toml` to `/etc/revad/revad.toml`, the default location where the reva binary will load its configuration.
Copy the `test_users.json` file to `/etc/revad/test_users.json` to match the configuration from `/etc/revad/revad.toml`.
Create directory where reva will log its outpout: `mkdir -p /var/log/revad`.

### Run revad
Ideally you would use an init system like systemd or docker to run it, for this tutorial we run it manually:
```
$ nohup revad  &
```

Let's take a look at the logs:

```
tail /var/log/revad/revad.log
```

### Connect to revad

The Reva daemon listens on port `9143` (configured in `/etc/revad/revad.toml`)
Let's use the reva client cli to connect to it:

```
$ reva -host localhost:9143 -insecure login basic
username: einstein
password: 
OK

$ reva whoami
```



