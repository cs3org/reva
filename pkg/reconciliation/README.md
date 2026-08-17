# Deploying the reconciliation jobs

The jobs span the shares and the public links, so they are wired as their own
serverless service rather than by one of the grpc share services. They go
through a share manager and a public share manager, named by driver like
everywhere else. Marking an item orphaned is not part of the CS3 API, so a
driver that does not implement it is refused at startup; today only `sql` does,
which is why it is the default for both.

Each job is enabled and configured on its own, so one can be left in dry-run, or
rescheduled, or pointed at another log, without touching the other.

```toml
[serverless.services.reconciliation]
jobs = ["orphan", "shallow"]
share_driver       = "sql"
publicshare_driver = "sql"
service_user_name  = "cboxreco"
service_user_uid   = 12345
service_user_gid   = 2766

[serverless.services.reconciliation.orphan]
schedule = "@daily"      # "@every <dur>" | "@hourly" | "@daily" | "@weekly"
dry_run  = false
log_file = "/var/log/revad/reconciliation-orphan.log"

[serverless.services.reconciliation.shallow]
schedule = "never"       # or a cadence, on-demand runs work either way
dry_run  = true
log_file = "/var/log/revad/reconciliation-shallow.log"
```

A job runs only if it is listed in `jobs`. `orphan` needs a `schedule`; on
`shallow` it is optional and also takes `never`, since that job can be triggered
on demand whether or not it has a cadence. `log_file` defaults to the path shown
above for each, and both drivers default to `sql`, so all four keys above the
service user can be omitted.

The drivers take their configuration from
`[serverless.services.reconciliation.share_drivers.<name>]` and
`[serverless.services.reconciliation.publicshare_drivers.<name>]`, the same keys
the usershareprovider and the publicshareprovider pass. The `sql` driver reads
its database settings from `[shared]`, so neither section is normally needed,
and neither is `jwt_secret`.

## Identity

The jobs runner hands a run a bare context, but we need a valid auth
so we can do stat's etc. Therefore, each run mints itself a token for
`service_user_name` and sends it with every call. The three `service_user_*`
keys are thus required and it needs to have the proper permissions on EOS
(for CERNBox, `cbox` does the trick).

## Jobs

`reconciliation.orphans` marks the shares and public links whose resource or
recipient no longer exists.

`reconciliation.shallow` writes the ACL entry each share implies onto its path,
when the storage has no entry or the wrong permissions. It only adds and corrects
entries. It never removes an ACL entry.

Start a run with `reva admin jobs run reconciliation.shallow [space=<space-id>]`.
Leave the space out to cover every space. This needs the admin API enabled. The
command prints a run id: `reva admin jobs status <run-id>` shows the state and
the totals, `reva admin jobs cancel <run-id>` stops the run.

With a `schedule`, the job also runs on a cadence over every space. That run has
its own name, so `reva admin jobs trigger reconciliation.shallow.scheduled`
starts it before its next fire.

A share below another share of the same recipient is redundant when it grants the
same access (`covered` in the totals) or less (`conflicting`). The job deletes
the row of such a share, which is what the share API does when the higher share
is created. The delete is soft, so you can undo it in the database. The ACL entry
on the path stays.

## Logs

Each job writes its own log (well, in fact, more like a journal), separate from revad's.
This is always written in JSON so that it is easy to parse by any tool in case we need
to revert certain actions.

An `event` is the job's own name followed by the step, so one job's lines never
have to be told apart from the other's by hand.

| event                             | meaning                                        |
| --------------------------------- | ---------------------------------------------- |
| `reconciliation.orphans.start`    | run started                                    |
| `reconciliation.orphans.mark`     | item marked orphaned (or would be, dry-run)    |
| `reconciliation.orphans.skip`     | item left untouched, lookup failed             |
| `reconciliation.orphans.fail`     | item was orphaned but the write failed         |
| `reconciliation.orphans.end`      | run totals                                     |
| `reconciliation.shallow.start`    | run started                                    |
| `reconciliation.shallow.grant`    | grant written to a path (or would be, dry-run) |
| `reconciliation.shallow.remove`   | redundant share removed (or would be, dry-run) |
| `reconciliation.shallow.skip`     | share left untouched, a lookup failed          |
| `reconciliation.shallow.fail`     | a grant or a removal was needed but failed     |
| `reconciliation.shallow.end`      | run totals                                     |

Every line also carries `job` and `run`, a uuid identifying the run, so the two
logs stay readable if both jobs are pointed at the same file.

`reconciliation.orphans.mark` carries `kind` (`share` or `publiclink`), `id`,
`reason`, `storage_id`, `opaque_id`, `owner`, `share_with`, `dry_run`. Revert
with `kind` and `id`.

`reconciliation.shallow.grant` carries `share`, `action` (`add` or `update`),
`path`, `storage_id`, `opaque_id`, `grantee`, `grantee_type`, `observed`,
`expected`, `dry_run`. Revert with `grantee` and `observed`; an empty `observed`
means the entry was added and has to be removed.

`reconciliation.shallow.remove` carries `share`, `path`, `storage_id`,
`opaque_id`, `grantee`, `grantee_type`, `level`, `reason`, `ancestor`,
`ancestor_path`, `ancestor_role`, `dry_run`. Revert by clearing `deleted_at` on
the row with id `share`.
