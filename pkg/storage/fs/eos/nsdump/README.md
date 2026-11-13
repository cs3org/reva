# nsdump

The deep reconciliation job compares the ACLs that should be on a space with
the ACLs that are really there. We get the first part from the shares and the
project roles. For the second part, we need the `sys.acl` of every file and
folder in the space.

We get this with `eos-ns-inspect`. This tool reads the namespace directly from
QuarkDB, so we do not need one call to the MGM per node. It prints every node
as a JSON object, with its xattrs, in one big JSON array.

The job runs in its own process, but only the EOS driver knows how to reach its
instance. So the storage provider runs the dump for the job: the job calls the
`reconciliation.dump` invocation over the control channel, and the provider
streams the nodes back in batches. The writes go the same way, one node at a
time. See `DumpNamespace`, `AddNodeACL` and `RemoveNodeACL` in `../reconcile.go`.

## What the driver gives the job

The driver turns each node into a `reconciliation.NSDumpInfo`:

- Users stay by uid, the way EOS stores them. The job resolves the users of
  the shares to uids, because there are far fewer shares than nodes.
- The permissions become a level (R, RW or D), with the same mapping
  `AddGrant` uses. So `rx!d` is R and `rwx+d` is RW.
- Only user (`u`) and group (`egroup`) entries go to the job. Other entry types
  cannot come from a share, so the job never touches them.
- A version folder (`.sys.v#.<file>`) gets `ACLsOf` set to its file. `AddGrant`
  writes the ACL of a file on its version folder too, so the job checks that
  copy against the shares of the file.
- The versions inside a version folder and the other `.sys.` nodes are left
  out. A share never sets an ACL there.
- A node whose `sys.acl` does not parse is left out too. We cannot fix what we
  cannot read.

## Clients

Each client implements `NSDumpClient`. `Setup` takes the config of the client
and `Dump` gives the entries under a root path, one at a time. All clients use
the same parser, so the only difference is where the JSON comes from.

The parser does not load the full array. It decodes one element, hands it on,
and then decodes the next one. This matters, because the dumps are big: for a
space with a million nodes, the JSON is about 800 MB, and the parsed entries
take another 300 MB. With streaming, the parse needs less than 1 MB. It is
about 15% slower, which is fine for a job that we start by hand.

Because the job compares while we parse, a dump that breaks part way gives an
error and no change set. Otherwise, we would apply the changes of only a part
of the space.

| Client | Source of the JSON | `nsdump_driver` |
|---|---|---|
| `EOSChunkedNSInspect` | runs `eos-ns-inspect` into a temp file | `chunked` (default) |
| `EOSMemoryNSInspect` | runs `eos-ns-inspect` into memory | `memory` |

Use `chunked`, unless the host has little disk and the spaces are small: then
`memory` saves the write to disk.

## Config

The keys are in the config of the EOS driver, next to `master_url`, because
they are different for each instance:

```toml
[grpc.services.storageprovider.drivers.eos]
master_url               = "root://eoshome-i01.cern.ch"
ns_inspect_quarkdb_nodes = ["eoshome-i01-qdb.cern.ch:7777"]
ns_inspect_password_file = "/keytabs/eoshome-i01_keytab"
ns_inspect_binary        = "/usr/bin/eos-ns-inspect"   # default
nsdump_driver            = "chunked"                   # default
nsdump_tmp_dir           = "/var/tmp/revad"            # default os.TempDir()
```

Without `ns_inspect_quarkdb_nodes` and `ns_inspect_password_file`, a dump fails. The
host of the storage provider needs the binary, the password file and network
access to the QuarkDB nodes.

### EOSChunkedNSInspect

This client runs the scan and writes the output to a temp file in
`nsdump_tmp_dir`. When the scan is done, we read the file back and parse it
entry by entry. So the full output is never in memory, only on disk.

Make sure this disk has room: a dump takes about 800 MB per million nodes. We
delete the file when the parse is done, also when it fails. If revad crashes
during a dump, the file stays, so you have to remove `nsdump-*.json` by hand.

### EOSMemoryNSInspect

This client keeps the full output in memory. When the command is done, we parse
it entry by entry, but the raw JSON stays in memory until the end.

Both clients run the same command:

```
eos-ns-inspect scan --path <root> [--maxdepth <n>] \
  --members <ns_inspect_quarkdb_nodes> --password-file <ns_inspect_password_file> \
  --json [--no-files]
```

`IgnoreFiles` adds `--no-files`, so we only get the folders. A `maxDepth` of 0
scans the full tree, and this is what the deep job does.
