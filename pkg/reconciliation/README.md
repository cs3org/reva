# Deploying the orphan job

The job spans the share and the public link tables, so it is wired as its own
serverless service rather than by one of the grpc share services. It reads and
writes those tables directly through gorm, so it always uses the `sql` driver.

```toml
[serverless.services.reconciliation]
schedule = "@daily"      # "@every <dur>" | "@hourly" | "@daily" | "@weekly"
dry_run  = false
log_file = "/var/log/revad/reconciliation.log"
```

`schedule` is required. The `[serverless.services.reconciliation.db]` section
takes the same keys as the `sql` share driver and falls back to `[shared]`, so
it can normally be omitted.

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
