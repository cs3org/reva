Bugfix: Lock source on move

When moving files until now only the lock of the targeted node would be checked.
This could lead to strange behaviour when using web editors like only office.
With checking the source nodes lock too, it is now forbidden to rename a file while it is locked

https://github.com/cs3org/reva/pull/3251
