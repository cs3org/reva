Bugfix: don't fetch groups for federated or guest accounts

Public-link guests and OCM users are not known to any user
backend, so every group lookup for them failed. With the REST
driver, each lookup was a failed request to the CERN
authorization service.

https://github.com/cs3org/reva/pull/6002
