Bugfix: honor pidfile passed with the -p option

The pidfile was always generated at a random path in the OS temp dir on
startup, ignoring the location passed with -p. As a result -s reload (and
the other -s signals) could not find the running master to signal it. The
-p path is now honored when starting revad.

https://github.com/cs3org/reva/pull/5653
