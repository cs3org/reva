Enhancement: Support property to enable health checking on a service

This update introduces a new service property called `ENABLE_HEALTH_CHECKS` that must be explicitly set to `true` if a service should be checked for its health status. This allows us to only enable these checks for partner sites only, skipping vendor sites.

https://github.com/cs3org/reva/pull/1347
