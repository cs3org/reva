Enhancement: Return linked-primary auth conflict HTTP contract

Protected HTTP auth middleware now returns a machine-readable 409 Conflict
response when gateway Authenticate surfaces CODE_ABORTED. The response includes
the linked-primary header and Libre Graph style error body so clients can show
dedicated linked-account blocked messaging.

https://github.com/cs3org/reva/pull/5642
