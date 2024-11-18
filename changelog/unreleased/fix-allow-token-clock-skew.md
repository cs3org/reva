Bugfix: Allow small clock skew in reva token validation

Allow for a small clock skew (3 seconds by default) when validating reva tokens
as the different services might be running on different machines.

https://github.com/cs3org/reva/pull/4955
https://github.com/cs3org/reva/issues/4952
