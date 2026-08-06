# Deploying the orphan job

The job spans the share and the public link tables, so it is wired as its own
serverless service rather than by one of the grpc share services. It reads and
writes those tables directly through gorm, so it always uses the `sql` driver.

```toml
[serverless.services.reconciliation]
service_user_name = "cboxreco"
service_user_uid  = 12345
service_user_gid  = 2766
schedule = "@daily"      # "@every <dur>" | "@hourly" | "@daily" | "@weekly"
dry_run  = false
log_file = "/var/log/revad/reconciliation.log"
```

`schedule` is required. `jwt_secret` and the
`[serverless.services.reconciliation.db]` section, which takes the same keys as
the `sql` share driver, both fall back to `[shared]`, so they can normally be
omitted.

## Identity

The jobs runner hands a run a bare context, so each run mints itself a token for
`service_user_name` and sends it with every call. The three `service_user_*`
keys are required and the account has to be a real one: EOS reads the ACLs of a
node as the caller before handing them out, and its driver refuses a caller
whose uid or gid is zero, so `root` does not work. If `skip_user_groups_in_token`
is set, the account also has to resolve in the user provider, since the auth
interceptor looks its groups up.

## Logs

The job writes its own log, separate from revad's, always JSON, one line per
action. `log_file` defaults to `/var/log/revad/reconciliation.log` and also
takes `stdout` or `stderr`. It is opened at startup and appended to, so an
unwritable path fails startup.

| event                   | meaning                                     |
| ----------------------- | ------------------------------------------- |
| `reconciliation.start`  | run started                                 |
| `reconciliation.orphan` | item marked orphaned (or would be, dry-run) |
| `reconciliation.skip`   | item left untouched, lookup failed          |
| `reconciliation.fail`   | item classified orphaned but write failed   |
| `reconciliation.end`    | run totals                                  |

Every line carries `run`, a uuid identifying the run it belongs to.

`reconciliation.orphan` carries `kind` (`share` or `publiclink`), `id`,
`reason`, `storage_id`, `opaque_id`, `owner`, `share_with`, `dry_run`. Revert
with `kind` and `id`.
