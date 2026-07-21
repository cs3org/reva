# Deploying the orphan job

The job is registered by the `usershareprovider` service, so it is configured
there. The share driver has to implement `reconciliation.ShareStore` (today the
`sql` one does); a schedule on a driver that does not fails at startup.

```toml
[grpc.services.usershareprovider.reconciliation]
schedule = "@daily"      # "@every <dur>" | "@hourly" | "@daily" | "@weekly"
dry_run  = false
```

Without `schedule` the job is not registered at all.
