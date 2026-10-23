Enhancement: Show the logs of a job run

A job run uses its run id as trace id and passes its job name to the services it calls, which log it as `job`.
`reva admin jobs logs <run-id>` shows the lines of one run from every instance, `-job <name>` those of every run of a job, and `-f` follows them.

https://github.com/cs3org/reva/pull/5928
