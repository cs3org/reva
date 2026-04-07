Bugfix: remove broken EOS token cache

There was a bug in the EOS token cache that caused infinite loops.
The token cache has been completely removed, since we will be moving
away from EOS tokens soon anyway.

https://github.com/cs3org/reva/pull/5570