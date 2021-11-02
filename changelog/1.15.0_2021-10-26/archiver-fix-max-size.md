Bugfix: Fix archiver max size reached error

Previously in the total size count of the files being
archived, the folders were taken into account, and this
could cause a false max size reached error because the
size of a directory is recursive-computed, causing the
archive to be truncated. Now in the size count, the
directories are skipped.

https://github.com/cs3org/reva/pull/2173
