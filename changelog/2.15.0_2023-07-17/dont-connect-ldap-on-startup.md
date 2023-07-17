Bugfix: Don't connect ldap on startup

This leads to misleading error messages. Instead connect on first request.

https://github.com/cs3org/reva/pull/4003
