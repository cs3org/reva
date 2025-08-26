Bugfix: sharing with guest accounts in spaces now works

We now fetch the users from the GW and use this info to create the share, instead of passing this info directly.
Additionally, we don't set `recursive` when setting an attribute on a file.

https://github.com/cs3org/reva/pull/5261