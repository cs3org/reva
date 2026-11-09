# nsdump

The deep reconciliation job compares the ACLs that should be on a space with
the ACLs that are really there. We get the first part from the shares and the
project roles. For the second part, we need the `sys.acl` of every file and
folder in the space.

We get this with `eos-ns-inspect`. This tool reads the namespace directly from
QuarkDB, so we do not need one call to the MGM per node. It prints every node
as a JSON object, with its xattrs, in one big JSON array. The job then walks
this dump entry by entry and compares each entry with the ACL tree.

## Clients

Each client implements `NSDumpClient`. `Setup` takes the config of the client
and `Dump` gives the entries under a root path, one at a time. All clients use
the same parser, so the only difference is where the JSON comes from.

The parser does not load the full array. It decodes one element, hands it to
the job, and then decodes the next one. This matters, because the dumps are
big: for a space with a million nodes, the JSON is about 800 MB, and the parsed
entries take another 300 MB. With streaming, the parse needs less than 1 MB.
It is about 15% slower, which is fine for a job that we start by hand.

Because the job compares while we parse, a dump that breaks part way gives an
error and no change set. Otherwise, we would apply the changes of only a part
of the space.

| Client | Source of the JSON | `nsdump_driver` |
|---|---|---|
| `EOSChunkedNSInspect` | runs `eos-ns-inspect` into a temp file | `chunked` (default) |
| `EOSMemoryNSInspect` | runs `eos-ns-inspect` into memory | `memory` |

You select the client of the deep job with `nsdump_driver`. Use `chunked`,
unless the host has little disk and the spaces are small: then `memory` saves
the write to disk.

### EOSChunkedNSInspect

This client runs the scan and writes the output to a temp file. When the scan
is done, we read the file back and parse it entry by entry. So the full output
is never in memory, only on disk.

The file goes in `tmp_dir`, which you set in the deep job section. If you do
not set it, we use `os.TempDir()`. Make sure this disk has room: a dump takes
about 800 MB per million nodes. We delete the file when the parse is done, also
when it fails. If revad crashes during a run, the file stays, so you have to
remove `nsdump-*.json` by hand.

```toml
[serverless.services.reconciliation.deep]
nsdump_driver = "chunked"
tmp_dir       = "/var/tmp/revad"
```

The command and the host requirements are the same as for
`EOSMemoryNSInspect`, see below.

### EOSMemoryNSInspect

This client runs the scan on the reva host and keeps the full output in memory.
When the command is done, we parse it entry by entry, but the raw JSON stays in
memory until the end. The command is:

```
eos-ns-inspect scan --path <root> [--maxdepth <n>] \
  --members <instance>-qdb:7777 --password-file /keytabs/<instance>_keytab \
  --json [--no-files]
```

For this to work, the host needs:

- `/usr/bin/eos-ns-inspect`
- the QuarkDB keytab of the instance in `/keytabs/<instance>_keytab`
- network access to `<instance>-qdb:7777`

`instance` is the EOS instance (for example `eoshome-i01`), not the alias in
the storage id. `ignorefiles` adds `--no-files`, so we only get the folders. A
`maxDepth` of 0 scans the full tree, and this is what the deep job does.
