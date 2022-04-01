Bugfix: Decomposed FS: return precondition failed if already locked

We've fixed the return code from permission denied to precondition failed if a
user tries to lock an already locked file.

https://github.com/cs3org/reva/pull/323
