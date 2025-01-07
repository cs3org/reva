Bugfix: Drop assumptions about user types when dealing with shares

We may have external accounts with regular usernames (and with null uid),
therefore the current logic to heuristically infer the user type from
a grantee's username is broken. This PR removes those heuristics and
requires the upper level to resolve the user type.

https://github.com/cs3org/reva/pull/4849
