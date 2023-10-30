Bugfix: Fix HTTP verb of the generate-invite endpoint

We changed the HTTP verb of the /generate-invite endpoint of the sciencemesh
service to POST as it clearly has side effects for the system, it's not just a
read-only call.

https://github.com/cs3org/reva/pull/4299
