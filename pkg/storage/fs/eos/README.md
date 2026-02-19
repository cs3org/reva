# EOS Storage Driver

This package implements a Storage Provider for [EOS](https://eos-docs.web.cern.ch/diopside/manual/index.html).

## What is EOS?

EOS is a distributed storage system built around a namespace service (MGM, or Management node) and storage servers (FSTs, FileStore Targets). The MGM handles all metadata and namespace operations; actual file data lives on FSTs and is accessed via the XRootD protocol (or HTTP). EOS provides versioning, a recycle bin, extended attributes, ACLs, quotas, and file locking natively.

## Package structure

```
pkg/storage/fs/eos/
├── eos.go               # Entry point: registers the "eos" driver, creates EosFS
├── client/
│   ├── eosclient.go     # EOSClient interface, shared types (FileInfo, Authorization, …)
│   ├── utils.go         # Version folder path helpers, attribute key utilities
│   ├── binary/
│   │   └── eosbinary.go # Client implementation: shells out to the `eos` CLI + xrdcopy
│   └── grpc/
│       ├── eosgrpc.go   # Client implementation: EOS gRPC for metadata
│       ├── eoshttp.go   # HTTP for data I/O (reads/writes to FSTs)
│       ├── acl.go       # ACL operations over gRPC
│       ├── attributes.go
│       ├── auth.go      # Token generation
│       ├── file_info.go
│       ├── file_ops.go
│       ├── quota.go
│       ├── recycle.go
│       └── versions.go
├── config.go             # Config struct with all tunables
├── eosfs.go              # Eosfs struct: init, path wrap/unwrap, convert, permission set
├── auth.go               # Auth resolution: UID/GID, tokens, daemon/single-user modes
├── file_ops.go           # Upload, Download, CreateDir, Move, Delete, ListFolder, …
├── file_info.go          # GetMD, GetPathByID, getPath (inode → path)
├── grant.go              # CS3 grant ↔ EOS ACL translation
├── locks.go              # File locking (app.lock + payload xattr)
├── recycle.go            # Recycle bin list / restore / purge
├── revisions.go          # File version list / download / restore
├── quota.go              # Quota reporting
└── arbitrary_metadata.go # user.* xattrs exposed as arbitrary metadata
```

## Architecture

### Client abstraction (`client/eosclient.go`)

The `EOSClient` interface decouples the filesystem logic from the transport used to reach EOS. Two implementations exist, selected at init time via the `use_grpc` config key:

|                  | Binary client                | gRPC client                          |
| ---------------- | ---------------------------- | ------------------------------------ |
| **Metadata ops** | Shells out to `/usr/bin/eos` | EOS gRPC API (`cern-eos/go-eosgrpc`) |
| **Data I/O**     | `xrdcopy` binary             | HTTP to FSTs via `eoshttp.go`        |
| **Auth**         | Keytab / uid-gid env vars    | Keytab or pre-shared auth key        |
| **Streaming**    | No                           | Yes (optional temp-file buffering)   |

The binary client was the original driver and remains useful when a full EOS installation is available on the same host. It is slower and less secure, but easier to set up. This can be used for demo's.

The gRPC client is a more modern approach, and is recommended for production usage. For production usage, it is also recommended to set up TLS. For this, point `http_client_certfile` and `http_client_keyfile` to the correct files. If you want to force TLS, set `allow_insecure` to false (this is the default). The gRPC client uses two separate channels: gRPC to the MGM for all namespace operations (stat, list, mkdir, rename, ACL, xattr, quota, recycle, versions) and HTTP to the FSTs for actual file data.

### Path handling

EOS paths are absolute on the cluster (`/eos/user/j/jsmith/…`). Storage providers operate on paths relative to the mount path, which is set up in the storage provider's config under `mount_path`. So, paths that arrive at the SP are relative to this mount path. On file operations, a namespace is wrapped / unwrapped:

- `wrap(ctx, path)` → prepends `conf.Namespace` to get the EOS-absolute path
- `unwrap(ctx, internal)` → strips `conf.Namespace` to get the SP-relative path

As an example, imagine that EOS has paths like `/eos/user/j/jsmith/…`, but you want to expose them as `/users/j/jsmith`. Then you would set the `mount_path` to `/users`. Requests come in at the SP with paths like `/j/jsmith`. The namespace should then be set to `/eos/user`, which means requests go out to EOS as `/eos/user/j/jsmith`. Responses from EOS are then "unwrapped" to become `/j/jsmith` again, before being prepended by the mount_path again (which is done by the Storage Provider).

The `UserLayout` template (e.g. `{{.Username}}` or `{{substr 0 1 .Username}}/{{.Username}}`) is used when constructing home directories from the namespace root.

### Authorization

Each call to EOS must carry authentication. The driver resolves the right auth on a per-request basis.

Rest of this: TODO after refactoring

### UID/GID resolution cache

EOS stores file ownership as integer UIDs. Reva and CS3 work with opaque string user IDs. The driver maintains a TTL cache (`userIDCache`, up to `user_id_cache_size` entries) that maps between the two, using the Reva gateway (`GetUser` / `GetUserByClaim`) as the source of truth.

On startup a background goroutine (`userIDcacheWarmup`) walks the namespace to `user_id_cache_warmup_depth` levels and pre-populates the cache, which is important for latency on large deployments.

### Versioning

EOS stores each version of a file in a hidden sibling folder named `.sys.v#.<filename>`. The driver:

- Filters these folders from directory listings (matched by `hiddenReg`).
- Exposes versions through `ListRevisions` / `DownloadRevision` / `RestoreRevision`.
- With `version_invariant: true` (the default), performs an extra stat to ensure the inode returned for a file always refers to the current version, not the version folder. This keeps clients that cache inodes consistent after an overwrite.

### Recycle bin

EOS recycle bins are per-user. The driver handles two cases:

- **User-owned spaces** — EOS recycle bin of the file's owner. The driver impersonates the owner when listing/restoring/purging.
- **Project spaces (ownerless)** — Identified by the `sys.forced.recycleid` EOS xattr. In this case a fixed `recycleid` is passed to the EOS recycle APIs rather than a user identity.

`ListRecycle` enforces a configurable date window (`max_days_in_recycle_list`) and entry count (`max_recycle_entries`) to prevent unbounded responses.

### ACLs and grants

EOS uses its own ACL syntax stored in the `sys.acl` extended attribute. The driver translates between EOS ACL entries and CS3 `Grant` objects:

- User grants → EOS `u:<uid>:<perms>` entries.
- Group grants → EOS `egroup:<name>:<perms>` entries.
- **Lightweight account grants** — EOS has no concept of lightweight users. These are stored as plain xattrs (`sys.reva.lwshare.<opaqueId> = <perms>`) and merged into the ACL evaluation at permission-check time.

Grant evaluation (`permissionSet`) also handles public share roles (viewer/uploader/editor) and OCM share roles, short-circuiting before inspecting ACLs.

### File locking

Locking uses two xattrs:

- `sys.app.lock` — The EOS-native lock attribute. EOS enforces this server-side, blocking writes from applications with a mismatched tag. Format: `expires:<unix>,type:shared,owner:<user>:<app>`.
- `sys.reva.lockpayload` — Base64-encoded JSON of the full CS3 `Lock` proto. Reva reads this to serve `GetLock`.

The EOS lock type is always `shared` (EOS does not have an exclusive lock), but is sufficient for WOPI-style app locking and checkout/checkin workflows.

The `app` parameter passed to every write and xattr operation is derived from the lock holder's app name, encoded as `http/reva_<appname>`. This ensures EOS's lock enforcement operates correctly.

### Data I/O (gRPC client)

Reads and writes go through `eoshttp.go`, which speaks directly to EOS FSTs over HTTPS using mutual TLS or an auth key. The base URL is the MGM URL (EOS redirects to the actual FST).

Two modes are supported for both directions:

- **Streaming** (default) — data flows directly between the caller and EOS without touching disk on the Reva host.
- **Local temp** (`read_uses_local_temp` / `write_uses_local_temp`) — data is buffered in `cache_directory` first. Required when the FST does not support HTTP chunked encoding on uploads.

### Chunked uploads (binary client / legacy)

The binary client handles the OwnCloud-style chunked upload protocol via `chunking.ChunkHandler`. Chunks are assembled on the local filesystem before being written to EOS. The gRPC client does not use this mechanism; chunked uploads should be disabled at the gateway level for gRPC deployments.

## Configuration reference

Key config options (see `fs/config.go` for the full list):

| Key                                              | Description                                                                    |
| ------------------------------------------------ | ------------------------------------------------------------------------------ |
| `namespace`                                      | EOS path prefix (e.g. `/eos/user/`)                                            |
| `quota_node`                                     | EOS path used for quota queries; defaults to `namespace`                       |
| `use_grpc`                                       | `true` → gRPC+HTTP client; `false` (default) → binary client                   |
| `master_grpc_uri`                                | gRPC endpoint of the EOS MGM (e.g. `eos-mgm.example.org:50051`)                |
| `master_url`                                     | XRootD/HTTP base URL (e.g. `root://eos-mgm.example.org`)                       |
| `grpc_auth_key`                                  | Shared secret for gRPC calls                                                   |
| `https_auth_key`                                 | Shared secret for HTTP data calls                                              |
| `keytab` / `use_keytab`                          | Kerberos keytab authentication                                                 |
| `user_layout`                                    | Go template for user home path (e.g. `{{substr 0 1 .Username}}/{{.Username}}`) |
| `version_invariant`                              | Stable inodes across file versions (default `true`)                            |
| `enable_home_creation`                           | Allow `CreateHome` calls; requires `create_home_hook`                          |
| `create_home_hook`                               | Path to a script invoked to provision new home directories                     |
| `force_single_user_mode` / `single_username`     | Impersonate one account for all calls                                          |
| `read_uses_local_temp` / `write_uses_local_temp` | Buffer I/O via `cache_directory`                                               |
| `user_id_cache_size`                             | Max entries in the UID ↔ CS3 user ID cache (default 1 000 000)                |
| `user_id_cache_warmup_depth`                     | Namespace walk depth on startup for cache warmup (default 2)                   |
| `max_recycle_entries`                            | Max items returned by `ListRecycle` (default 2000)                             |
| `max_days_in_recycle_list`                       | Max date span for `ListRecycle` (default 14 days)                              |

## Unsupported operations

The following `storage.FS` methods return `errtypes.NotSupported`:

- `GetHome` — home path is handled by the spaces registry, not this driver.
- `CreateStorageSpace` / `ListStorageSpaces` / `UpdateStorageSpace` — spaces are managed by an external spaces registry.
- `EmptyRecycle` — only targeted purge of individual items is supported.

## EOS documentation

- [EOS manual (Diopside)](https://eos-docs.web.cern.ch/diopside/manual/index.html)
- [EOS gRPC API](https://github.com/cern-eos/go-eosgrpc)
