Enhancement: share hierarchy checks

This PR adds a hierarchical checking algorithm for shares to the gateway,
as defined in ADR general/0005-sharing. Concrectely, the new algorithm does
the following: 

* Before applying any ACL, the gateway checks for parent and child shares in 
  the database.
* Based on their relationships and permission levels, the gateway decides whether
  to apply, reapply, or reject the operation.
* ACL updates will be applied orderd by path-length (where the shortest comes
  first) to maintain  consistent inheritance semantics (otherwise, you would 
  overwrite child shares).
* The algorithm applies equally to create, update, and delete operations.

https://github.com/cs3org/reva/pull/5562