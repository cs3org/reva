Enhancement: Add embedded processing failure mode

Embedded transfers may fail due to a number of reasons, an internal service could be down
or the remote repository could be unreachable. Instead of being stuck in the "Transferring" 
state, shares are now put in the "Rejected" state so that this can be surfaced to the user
in a reasonable way.

https://github.com/cs3org/reva/pull/5826