Bugfix: preserve child ACLs when creating parent shares

When a parent share made descendant share records redundant, the gateway removed
the descendant storage grants after applying the parent grant. For recursive ACL
backends this could remove the parent ACL from the descendant path again. The
gateway now only removes the redundant child share records and leaves the
storage ACLs produced by the parent grant intact.
