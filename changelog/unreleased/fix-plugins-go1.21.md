Bugfix: Fix plugin's registration when reva is built with version 1.21

With go 1.21 the logic for package initialization has changed,
and the plugins were failing in the registration.
Now the registration of the plugins is deferred in the main.

https://github.com/cs3org/reva/pull/4113
