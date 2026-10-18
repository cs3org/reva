Enhancement: record withdrawn OCM grants durably in the SQL share driver

The SQL OCM share driver now stores the identity of each provider grant,
keyed by the original sender's domain and the remote share ID, and binds
received shares to it. Historical shares are bound at startup only when
their sender domain is known and their type mapping is consistent; the
others stay unbound and are never guessed. A new repository call records a
withdrawal by the provider: it keeps every share of that grant as invalid
and hidden, with its protocols, and records the withdrawal time once.

Every writer of received shares takes the grant lock, on MySQL with row
locks and on SQLite with immediate transactions, so a withdrawn grant cannot
be accepted, unhidden, transferred or created again, even after a restart or
from another process; such attempts fail with a precondition error that OCS
and Graph report as 409 conflicts. A transfer that completes after a
withdrawal changes nothing. Reading or launching an invalid received share
is refused before any cached credential, discovery or token exchange is
used, including on retries. The JSON driver does not support withdrawal.
