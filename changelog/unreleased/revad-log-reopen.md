Enhancement: Reopen the log files on SIGUSR2

revad now reopens its log files when it receives SIGUSR2 (or `revad -s
reopen`). Logs can then be rotated by renaming them, e.g. logrotate with
`create` and a postrotate signal, instead of `copytruncate`, which drops the
lines written while the copy is synced to disk and leaves log shippers that
follow the file to notice the truncation.

https://github.com/cs3org/reva/pull/5932
