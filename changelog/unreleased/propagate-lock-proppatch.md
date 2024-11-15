Bugfix: Propagate lock in PROPPATCH
Clients using locking (ie. Windows) could not create/copy files over webdav as file seemed to be locked.