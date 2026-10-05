Bugfix: Harden the json public share manager against races, panics and leaks

The json public share manager (pkg/publicshare/manager/json) and its cs3
persistence backend received a round of concurrency and lifecycle fixes:

- The manager's mutex is now a sync.RWMutex, and every read-only operation
  (Dump, GetPublicShare, ListPublicShares, GetPublicShareByToken) takes an
  RLock instead of a full Lock, so concurrent reads no longer serialize
  behind each other.
- Every write path (Load, CreatePublicShare, UpdatePublicShare,
  RevokePublicShare, the janitor) now mutates its own copy of the share
  database instead of the map returned by Read, since that map can alias
  the persistence layer's cache and be shared with concurrent readers. The
  in-memory and cs3 persistence backends were adjusted to match this
  copy-on-write contract.
- The cs3 persistence backend's one-time initialization flag is now an
  atomic.Bool guarded by its own mutex, so already-initialized callers no
  longer contend with each other or with in-flight reads for a single lock.
  Its mtime-based read cache is now itself guarded by a mutex, and a
  successful Write invalidates that cache so a subsequent Read is forced to
  refetch instead of serving a stale entry.
- The janitor no longer subscribes to OS signals (SIGHUP/SIGINT/SIGQUIT) to
  know when to stop. The manager now exposes a Close(ctx) method
  (publicshare.ClosableManager) that cancels the janitor's context and waits
  for its goroutine to exit; the publicshareprovider gRPC service calls it
  on shutdown with a bounded timeout instead of doing nothing.
- GetPublicShare, ListPublicShares and GetPublicShareByToken no longer
  revoke an expired share inline while serving a read; they just treat it
  as not found/skip it. Actual deletion is left to the janitor, which now
  removes expired entries directly instead of recursively calling back into
  RevokePublicShare.
- Unchecked type assertions on the persisted "share" and "password" fields
  (e.g. v.(map[string]interface{})["share"].(string)) were replaced by a
  helper that reports failure instead of panicking, so a single malformed
  or legacy entry in publicshares.json no longer crash-loops the janitor or
  fails a request. The "password" field is treated as optional, since
  shares without password protection never had one to begin with.
- The default janitor run interval changed from 60 seconds to 1 hour.

https://github.com/owncloud/reva/pull/758
