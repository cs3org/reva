Enhancement: Allow for rolling back migrations

The decomposedfs now supports rolling back migrations (starting with 0004). It
also got a Migrations() method which returns the list of migrations incl. their
states.

https://github.com/cs3org/reva/pull/4083
