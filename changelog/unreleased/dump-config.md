Enhancement: Dump reva config on SIGUSR1

Add an option to the runtime to dump the configuration on a file
(default to `/tmp/reva-dump.toml` and configurable) when the process
receives a SIGUSR1 signal. Eventual errors are logged in the log.

https://github.com/cs3org/reva/pull/4031
