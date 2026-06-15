# Background jobs

The `jobs` serverless service runs background work in reva: tasks that happen
on a schedule (e.g. warming a cache, cleaning up expired state) and tasks that
happen once on demand (e.g. on a user request). The service is a thin host; the
framework itself lives in [`pkg/rjobs`](../../../../pkg/rjobs).

## Architecture

There is one **runner** per process. It owns scheduling, a worker pool and the
job lifecycle. Jobs are *registered* into the framework by the code that owns
them; the runner discovers them at startup and drives them.

A job is one of two kinds:

- **Periodic** — runs on a schedule. Declared with a required `Scope`:
  - `ScopeAllNodes`: runs on **every** process, as a local in-memory ticker.
    Never touches NATS. Use it for replica-local state, e.g. warming a cache.
  - `ScopeLeader`: runs on **exactly one** process per tick. Goes through the
    durable queue. Use it for work that mutates shared state and must happen
    once, e.g. a cleanup.
- **On-demand** — runs once when something calls `Enqueue`. Always durable.

Durable work (on-demand and `ScopeLeader` periodic) is backed by **NATS
JetStream**: a work-queue stream for the runs, plus a key-value bucket holding
each periodic job's next-fire time. Per-run **status** is written to a **SQL**
table and can be read back by run id. `ScopeAllNodes` jobs need neither — they
are pure local tickers, so they keep working even if NATS is down.

```
                 register (in-process, at startup)
                              │
        ┌─────────────────────┴─────────────────────┐
        ▼                                            ▼
  periodic jobs                                on-demand jobs
   │        │                                        │
ScopeAll  ScopeLeader ───────────┐                   │ Enqueue()
 Nodes        │ scheduler         ▼                   ▼
   │          └──────────►  NATS work queue  ◄────────┘
local ticker                      │ Claim()
(no NATS)                         ▼
                              worker pool ──► Job runs ──► SQL status
```

### Multiple processes

Several processes can run the `jobs` service against the same NATS and status
DB without duplicating work:

- The **scheduler** runs on every process. The next-fire advance in the KV
  bucket is atomic, so exactly one process wins a given tick and enqueues the
  run. No leader election is involved.
- The **work queue** delivers each run to a single worker. Crucially, a process
  only subscribes to the jobs it has **registered**, so it never claims a run
  for a job it does not know about; that run waits for a process that does.

This means each process must register the jobs it is expected to run. A run for
a job that no process has registered simply stays in the queue.

### Delivery semantics

Delivery is **at-least-once**: a run may execute more than once (e.g. after a
crash, via the queue's redelivery). Jobs must therefore be **idempotent**.

## Using it as a developer

You write a job in your own package and register it. Two ingredients:

1. register the job (at `init`, or at construction time for periodic jobs that
   capture live dependencies), and
2. make sure your package is imported so the `init` runs (a blank import in the
   relevant loader, like the other services).

### A periodic job

`Run` is a closure, so it can capture whatever the job needs (a cache handle, a
client, ...). Pick the scope deliberately — it decides the multi-process
behaviour.

```go
// in your component's constructor, once its dependencies exist:
err := rjobs.RegisterPeriodic(rjobs.Periodic{
    Name:       "mycomponent.warm_cache",
    Schedule:   "@every 5m",          // "@every <dur>" | "@hourly" | "@daily" | "@weekly"
    Scope:      rjobs.ScopeAllNodes,  // per-process cache => run everywhere
    RunOnStart: true,                 // prime at boot instead of waiting a tick
    Run: func(ctx context.Context) error {
        return cache.Warm(ctx)
    },
})
```

Use `ScopeLeader` instead when the work mutates shared state and must run once
across the cluster (a `ScopeLeader` job needs NATS configured).

### An on-demand job

Register a constructor by name; the framework builds the job and calls `Run`
with the per-run parameters. The returned `Params` are stored as the run's
result.

```go
func init() {
    _ = rjobs.RegisterOnDemand("mycomponent.export", New)
}

func New(ctx context.Context, m map[string]any) (rjobs.Job, error) {
    return &exportJob{ /* ... */ }, nil
}

func (j *exportJob) Run(ctx context.Context, p rjobs.Params) (rjobs.Params, error) {
    // p carries the parameters passed to Enqueue.
    // return a result payload (or nil); return an error to have the run retried.
    return rjobs.Params{"url": downloadURL}, nil
}
```

### Triggering an on-demand job

Any in-process caller (e.g. an HTTP handler) enqueues a run through the
process-wide runner. `Enqueue` is durable: it returns once the run is
persisted, and returns a `RunID`.

```go
runner := rjobs.Default()       // nil if the jobs service is not enabled
if runner == nil {
    // jobs not available in this deployment
}

runID, err := runner.Enqueue(ctx, "mycomponent.export", rjobs.Params{
    "user_id": uid,
}, rjobs.WithIdempotencyKey("export:"+uid)) // collapse duplicate requests
```

`Enqueue` is the seam a future RPC service would wrap; today it is in-process
only.

### Checking a run's status

Use the `RunID` to read status back:

```go
st, err := rjobs.Default().Status(ctx, runID)
// st.State is queued | running | succeeded | failed
// st.Result holds the payload the job returned on success
// st.LastError holds the error of the last failed attempt
```

Note that `failed` is **not terminal**: a failed run is retried, so `failed`
means "the last attempt failed, another is coming".

## Configuration

```toml
[serverless.services.jobs]
worker_pool_size = 4
nats_address     = "nats:4222"   # omit to run only ScopeAllNodes jobs
nats_prefix      = "reva-jobs"

[serverless.services.jobs.status_db]
db_engine   = "mysql"
db_username = "reva"
db_password = "reva"
db_host     = "mysql"
db_port     = 3306
db_name     = "reva_jobs"
```

Without `nats_address` the runner still starts, but only `ScopeAllNodes`
periodic jobs run; on-demand and `ScopeLeader` jobs need the queue and the
status DB.
