Enhancement: let a service act on the Admin API

`admin.AsService` gives a reva service a short-lived token for the synthetic user `service:<name>`, with only the admin scope, so it can call the Admin API and appears under its own name in the audit log.

https://github.com/cs3org/reva/pull/5914
