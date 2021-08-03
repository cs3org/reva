Bugfix: Do not overwrite more specific matches when finding storage providers

Depending on the order of rules in the registry it could happend that more specific matches (e.g. /home/Shares) were
overwritten by more general ones (e.g. /home). This PR makes sure that the registry always returns the most specific
match.

https://github.com/cs3org/reva/pull/1937