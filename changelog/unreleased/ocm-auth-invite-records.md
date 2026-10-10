Bugfix: guard share and invite authentication records

We've hardened the OCM invitation and direct-secret share authentication
paths against malformed records. Direct-secret share lookup now rejects
malformed share records, grantees, creators and access methods with a
constant invalid-credentials diagnostic instead of panicking, and reports
the accepted-user status message on not-found. The invite manager
validates requests and context identities before storage access, and all
three invite repositories reject invalid input, skip corrupt stored rows,
and no longer panic on the memory driver's first accepted-user insert.
