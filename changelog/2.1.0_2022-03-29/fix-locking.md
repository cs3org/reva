Bugfix: Fix locking on publik links and the decomposed filesystem

We've fixed the behavior of locking on the decomposed filesystem, so that now
app based locks can be modified user independently (needed for WOPI integration).
Also we added a check, if a lock is already expired and if so, we lazily delete the lock.
The InitiateUploadRequest now adds the Lock to the upload metadata so that an upload to an
locked file is possible.

We'v added the locking api requests to the public link scope checks, so that locking
also can be used on public links (needed for WOPI integration).

https://github.com/cs3org/reva/pull/2625
