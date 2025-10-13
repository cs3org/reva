Bugfix: Searching for federated users

Since the "sm:" prefix is no longer given by the frontend when searching
for federated users this is removed and the if is replaced with a flag to 
perform the search only when OCM is enabled.

https://github.com/cs3org/reva/pull/5350


