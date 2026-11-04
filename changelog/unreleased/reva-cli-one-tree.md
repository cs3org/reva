Enhancement: one reva CLI for users and operators

The reva CLI has one command tree: a command uses the Admin API when its target or options need it, and `--as`
impersonates a user. Commands and flags have the names of cernbox-cli, Kerberos login reads the ticket cache, and
plugins can add commands at build time. `reva invoke` checks and completes the arguments of an invocation.

https://github.com/cs3org/reva/pull/5960
