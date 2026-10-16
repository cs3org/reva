# Reminders

The `reminders` serverless service tells users about a condition on their
data, for example a public link that expires soon. Each condition is a *kind*,
in [`pkg/reminders`](../../../../pkg/reminders): it registers in
`pkg/reminders/registry` and is compiled in by `pkg/reminders/loader`.

A kind yields the subjects whose condition holds, each with a *level* (higher
is more urgent), and builds the notification of one recipient. The service
remembers, per subject, the highest level that it sent while the condition
holds:

- a level above the level sent: the recipient is told, once;
- the same level or lower: nothing;
- level 0: the episode stays open, nobody is told;
- a subject that a complete scan does not yield: the episode closes, and the
  subject starts again from the lowest level when it comes back.

The ledger is three SQL tables: `reminders_state` (the open episodes),
`reminders_sent` (the history) and `reminders_runs` (the last run of each
kind). The notifications go through the gateway's `PublishEvent`, so the
`notifications` service needs a rule and a template for the event of each
kind.

## Configuration

The service goes in a process that runs the `jobs` service. Each configured
kind is the job `reminders.<kind>`, on demand, and `reminders.<kind>.scheduled`
when it has a schedule.

```toml
[serverless.services.reminders]
service_user_name = "cbox"   # the identity of the runs, known to the storage
service_user_uid = 58679
service_user_gid = 1028
# mail_domain = "cern.ch"    # <name>@<mail_domain> when the directory has no mail
# retention_days = 365       # history kept by the reminders.purge job
# jwt_secret and db_* fall back to [shared]

[serverless.services.reminders.kinds.<kind>]
schedule = "@daily"          # empty: on demand only
dry_run = false              # log what would be sent, change nothing
# run_on_start = false
# the other keys belong to the kind
```

## Operations

```
reva admin reminders                          one row per kind
reva admin reminders state [-kind] [-recipient] [-subject]
reva admin reminders history [-kind] [-recipient] [-subject] [-since 7d]
reva admin reminders check <kind>             what a run would send now
reva admin reminders forget <kind> <subject>  tell this subject again
reva admin jobs run reminders.<kind>          run a kind now
```
