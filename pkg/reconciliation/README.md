# Deploying the reconciliation jobs

The jobs span the share and the public link tables, so they are wired as their
own serverless service rather than by one of the grpc share services. They read
and write those tables directly through gorm, so they always use the `sql`
driver.

Each job is enabled and configured on its own, so one can be left in dry-run, or
rescheduled, or pointed at another log, without touching the other.

```toml
[serverless.services.reconciliation]
jobs = ["orphan", "shallow"]
service_user_name = "cboxreco"
service_user_uid  = 12345
service_user_gid  = 2766

[serverless.services.reconciliation.orphan]
schedule = "@daily"      # "@every <dur>" | "@hourly" | "@daily" | "@weekly"
dry_run  = false
log_file = "/var/log/revad/reconciliation-orphan.log"

[serverless.services.reconciliation.shallow]
schedule = "@weekly"
dry_run  = true
log_file = "/var/log/revad/reconciliation-shallow.log"
```

A job runs only if it is listed in `jobs`, and every listed job needs a
`schedule`. `log_file` defaults to the path shown above for each. `gatewaysvc`,
`jwt_secret` and the `[serverless.services.reconciliation.db]` section, which
takes the same keys as the `sql` share driver, all fall back to `[shared]`, so
they can normally be omitted.

## Identity

The jobs runner hands a run a bare context, but we need a valid auth
so we can do stat's etc. Therefore, each run mints itself a token for
`service_user_name` and sends it with every call. The three `service_user_*`
keys are thus required and it needs to have the proper permissions on EOS
(for CERNBox, `cbox` does the trick).

## Jobs

`reconciliation.orphans` marks the shares and public links whose resource or
recipient no longer exists.

`reconciliation.shallow` writes the ACL entry each non-orphan share implies onto
its path when the storage has none or has the wrong permissions. It visits only
shared paths, and only ever adds or corrects an entry, never removes one.

Which entries have to exist is decided by `sharehierarchy.CheckGrantConsistency`,
the same check that runs when a share is created: an entry is only written where
it escalates beyond every share above it, per recipient and per space. A share
that grants exactly what an ancestor grants is inherited and counted `covered`.
One that grants anything else is a share the check would not have created, and
writing its entry would take away access the ancestor grants, so it is counted
`conflicting` and logged as `reconciliation.shallow.skip` with
`reason=shadowed-by-ancestor` and the ancestor's id, path and role.

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
| `reconciliation.shallow.skip`     | share left untouched, see `reason`             |
| `reconciliation.shallow.fail`     | grant was needed but the write failed          |
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
