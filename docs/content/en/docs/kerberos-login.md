---
title: "Kerberos login"
linkTitle: "Kerberos login"
weight: 60
description: Sign in to reva with the CLI and a Kerberos ticket
---

`reva login --method kerberos` signs in with the ticket that `kinit` stored, and
asks for no password. The CLI reads the ticket from a `FILE:` credential cache,
builds a SPNEGO token for the service principal (SPN) of the server, and sends it
to the gateway in `Authenticate` with the type `kerberos`. The `kerberos` auth
manager checks the token against its keytab and resolves the user from the
ticket.

```sh
kinit
reva login --method kerberos --spn HTTP/cernbox.cern.ch
reva admin elevate
```

## Settings

- **Credential cache:** `--ccache`, else `$KRB5CCNAME`, else `/tmp/krb5cc_<uid>`.
  The CLI reads only `FILE:` caches. If `krb5.conf` keeps the tickets in a
  `KEYRING:` or `KCM:` cache, run `KRB5CCNAME=FILE:/tmp/krb5cc_$(id -u) kinit`.
- **Kerberos config:** `$KRB5_CONFIG`, else `/etc/krb5.conf`. The CLI needs it
  only when the cache has no ticket for the SPN yet.
- **SPN:** `--spn`, else `$REVA_KERBEROS_SPN`, else
  `reva config set kerberos-spn HTTP/<host>`, else `HTTP/<gateway host>`.

## Server

The gateway routes the type `kerberos` to an auth provider with the `kerberos`
auth manager:

```toml
[grpc.services.authregistry.drivers.static.rules]
kerberos = "localhost:19000"

[grpc.services.authprovider]
auth_manager = "kerberos"

[grpc.services.authprovider.auth_managers.kerberos]
keytab = "/etc/reva/http.keytab"
service_principal = "HTTP/reva.test"
realm = "TEST.REVA"
```

The client does not ask for mutual authentication, because the auth provider
cannot send a reply token back.

## Try it with a local KDC

MIT Kerberos (`krb5kdc`, `kdb5_util`, `kadmin.local`, `kinit`) can run a
test realm as a normal user. Put the files in an empty directory `$K`.

`$K/krb5.conf`:

```ini
[libdefaults]
 default_realm = TEST.REVA
 rdns = false
[realms]
 TEST.REVA = {
  kdc = 127.0.0.1:18888
 }
[domain_realm]
 reva.test = TEST.REVA
```

`$K/kdc.conf` (replace `$K` with the directory):

```ini
[kdcdefaults]
 kdc_ports = 18888
 kdc_tcp_ports = 18888
[realms]
 TEST.REVA = {
  database_name = $K/principal
  key_stash_file = $K/stash
  acl_file = $K/kadm5.acl
  supported_enctypes = aes256-cts-hmac-sha1-96:normal
 }
```

Then create the realm, a user, and the keytab of the service, and start the KDC:

```sh
export KRB5_CONFIG=$K/krb5.conf KRB5_KDC_PROFILE=$K/kdc.conf
touch $K/kadm5.acl
kdb5_util create -s -r TEST.REVA -P masterpw
kadmin.local -r TEST.REVA -q "addprinc -pw alicepw alice"
kadmin.local -r TEST.REVA -q "addprinc -randkey HTTP/reva.test"
kadmin.local -r TEST.REVA -q "ktadd -k $K/http.keytab HTTP/reva.test"
krb5kdc -r TEST.REVA -P $K/kdc.pid
```

Start revad with the server config above, `keytab = "$K/http.keytab"`, and a
user provider that knows `alice` (for the gRPC gateway on a test machine, also
set `disable_home_creation_on_login = true` if there is no storage). Then:

```sh
export KRB5CCNAME=FILE:$K/cc
echo alicepw | kinit alice
reva --insecure --gateway localhost:19000 login --method kerberos --spn HTTP/reva.test
reva --insecure --gateway localhost:19000 whoami
```

To stop the KDC, run `kill $(cat $K/kdc.pid)`.
