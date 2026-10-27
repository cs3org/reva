---
title: Quick start
description: Run a single reva server with Docker, log in, and upload a file over WebDAV and with the reva CLI.
weight: 10
---

This guide starts one `revad` process that holds all the services for a small setup. It stores files on the local disk and reads users from a JSON file. Use it to learn reva, not in production.

You need [Docker](https://docs.docker.com/get-docker/) (or Podman) and `curl`.

## 1. Write the configuration

Make a folder and save these two files in it.

{{< file "quickstart/revad.toml" >}}

{{< file "quickstart/users.json" >}}

The configuration enables these services. Each name links to its reference page.

| Table | What it does |
| --- | --- |
| [`gateway`](/docs/components/grpc-services/gateway/) | Receives all CS3 API calls and routes them. |
| [`authregistry`](/docs/components/grpc-services/authregistry/), [`authprovider`](/docs/components/grpc-services/authprovider/) | Check passwords with the [`json` auth manager](/docs/components/drivers/auth-managers/json/). |
| [`userprovider`](/docs/components/grpc-services/userprovider/) | Looks up users with the [`json` user manager](/docs/components/drivers/user-managers/json/). |
| [`storageregistry`](/docs/components/grpc-services/storageregistry/), [`storageprovider`](/docs/components/grpc-services/storageprovider/) | Serve each user's home folder with the [`localhome` driver](/docs/components/drivers/storage-drivers/localhome/). |
| [`datagateway`](/docs/components/http-services/datagateway/), [`dataprovider`](/docs/components/http-services/dataprovider/) | Move file data in and out of the storage. |
| [`ocdav`](/docs/components/http-services/ocdav/) | Serves WebDAV. |

## 2. Start the server

Run this command in the folder with the two files:

```shell
docker run --rm --name reva \
  -p 19000:19000 -p 19001:19001 \
  -v "$PWD/revad.toml:/etc/revad/revad.toml:ro" \
  -v "$PWD/users.json:/etc/revad/users.json:ro" \
  cs3org/revad:v{{< version >}}
```

Port 19000 is gRPC (the gateway). Port 19001 is HTTP (WebDAV and data transfers). The log shows `http server listening` and `grpc server listening` when the server is ready.

{{< callout >}}
On SELinux hosts, such as Fedora or RHEL, add `,Z` after `ro` in both `-v` options.
{{< /callout >}}

## 3. Use WebDAV

Upload a file as the user `einstein`, then read it back and list the home folder:

```shell
echo "hello reva" > hello.txt
curl -u einstein:relativity -T hello.txt http://localhost:19001/remote.php/webdav/hello.txt
curl -u einstein:relativity http://localhost:19001/remote.php/webdav/hello.txt
curl -u einstein:relativity -X PROPFIND -H "Depth: 1" http://localhost:19001/remote.php/webdav/
```

The first command returns HTTP 201. The file is now in the container, under `/var/tmp/reva/data`.

Any WebDAV client can connect to `http://localhost:19001/remote.php/webdav/` with the same user name and password.

## 4. Use the reva CLI

The `reva` command line client talks to the gateway over gRPC. Download it from the [release page](https://github.com/cs3org/reva/releases/tag/v{{< version >}}). For Linux on x86-64:

```shell
curl -Lo reva https://github.com/cs3org/reva/releases/download/v{{< version >}}/reva_v{{< version >}}_linux_amd64
chmod +x reva
```

Log in, then work with files. The `--insecure` flag is necessary because this server does not use TLS.

```shell
./reva config set host localhost:19000
echo relativity | ./reva --insecure login --username einstein basic
./reva --insecure whoami
./reva --insecure ls /home
./reva --insecure upload hello.txt /home/notes.txt
./reva --insecure download /home/notes.txt copy.txt
```

Run `./reva --help` to see all commands, such as `share`, `public-share`, and `versions`.

## 5. Stop the server

```shell
docker stop reva
```

The container is removed when it stops, together with the uploaded files.

## Run without Docker

Download `revad` from the [release page](https://github.com/cs3org/reva/releases/tag/v{{< version >}}), or build it:

```shell
git clone https://github.com/cs3org/reva
cd reva
make revad
```

Change `/etc/revad/users.json` in `revad.toml` to the path of your `users.json`, then start it:

```shell
./cmd/revad/revad -c revad.toml
```

Use `revad -t -c revad.toml` to check a configuration without starting the server.

## Next steps

- Read the [architecture](/docs/concepts/architecture/) to see how the services work together.
- Read [configuration](/docs/concepts/configuration/) to split services across processes.
- Replace the drivers: [EOS](/docs/components/drivers/storage-drivers/eos/) or [CephFS](/docs/components/drivers/storage-drivers/cephfs/) for storage, [LDAP](/docs/components/drivers/auth-managers/ldap/) or [OIDC](/docs/components/drivers/auth-managers/oidc/) for login.
