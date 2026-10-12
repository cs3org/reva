Enhancement: app passwords with a client kind, permissions and an expiry

App passwords are stored with a client kind, permissions and roots, can be sent alone as a bearer token, and are checked against their kind's presentation, user agents and paths.
Existing app passwords become sync app passwords with every permission; the login flow mints sync or CLI app passwords.

https://github.com/cs3org/reva/pull/5895
