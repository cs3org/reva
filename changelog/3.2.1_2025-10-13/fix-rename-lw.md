Bugfix: fix permission check for MOVE for LW accs

The permission check for MOVE requests for lightweight accounts
was broken: it checked whether the user has permission on the destination
(which does not exist yet), instead of the destination's parent

https://github.com/cs3org/reva/pull/5337
