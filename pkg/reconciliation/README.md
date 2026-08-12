# Deploying the reconciliation jobs

The jobs span the shares and the public links, so they are wired as their own
serverless service rather than by one of the grpc share services. They go
through a share manager and a public share manager, named by driver like
everywhere else. Marking an item orphaned is not part of the CS3 API, so a
driver that does not implement it is refused at startup; today only `sql` does,
which is why it is the default for both.

Each job is enabled and configured on its own, so one can be left in dry-run, or
rescheduled, or pointed at another log, without touching the others. For now
there is only the orphan job.

```toml
[serverless.services.reconciliation]
jobs = ["orphan"]
share_driver       = "sql"
publicshare_driver = "sql"
service_user_name  = "cboxreco"
service_user_uid   = 12345
service_user_gid   = 2766

[serverless.services.reconciliation.orphan]
schedule = "@daily"      # "@every <dur>" | "@hourly" | "@daily" | "@weekly"
dry_run  = false
log_file = "/var/log/revad/reconciliation-orphan.log"
```

A job runs only if it is listed in `jobs`, and every listed job needs a
`schedule`. `log_file` defaults to the path shown above, and both drivers
default to `sql`, so all four keys above the service user can be omitted.

The drivers take their configuration from
`[serverless.services.reconciliation.share_drivers.<name>]` and
`[serverless.services.reconciliation.publicshare_drivers.<name>]`, the same keys
the usershareprovider and the publicshareprovider pass. The `sql` driver reads
its database settings from `[shared]`, so neither section is normally needed,
and neither are `gatewaysvc` and `jwt_secret`.

## Identity

The jobs runner hands a run a bare context, but we need a valid auth
so we can do stat's etc. Therefore, each run mints itself a token for
`service_user_name` and sends it with every call. The three `service_user_*`
keys are thus required and it needs to have the proper permissions on EOS
(for CERNBox, `cbox` does the trick).

## Jobs

`reconciliation.orphans` marks the shares and public links whose resource or
recipient no longer exists.

## Logs

Each job writes its own log (well, in fact, more like a journal), separate from revad's.
This is always written in JSON so that it is easy to parse by any tool in case we need
to revert certain actions.

An `event` is the job's own name followed by the step, so one job's lines never
have to be told apart from another's by hand.

| event                           | meaning                                     |
| ------------------------------- | ------------------------------------------- |
| `reconciliation.orphans.start`  | run started                                 |
| `reconciliation.orphans.mark`   | item marked orphaned (or would be, dry-run) |
| `reconciliation.orphans.skip`   | item left untouched, lookup failed          |
| `reconciliation.orphans.fail`   | item classified orphaned but write failed   |
| `reconciliation.orphans.end`    | run totals                                  |

Every line also carries `job` and `run`, a uuid identifying the run it belongs
to, so the logs stay readable if several jobs are pointed at the same file.

`reconciliation.orphans.mark` carries `kind` (`share` or `publiclink`), `id`,
`reason`, `storage_id`, `opaque_id`, `owner`, `share_with`, `dry_run`.
