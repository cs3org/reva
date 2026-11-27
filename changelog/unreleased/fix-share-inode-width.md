Bugfix: Store share inodes longer than 32 characters

The share manager stores the resource opaque id in the `inode` column of the
`shares`, `public_links` and `labels` tables. The GORM model declared those
columns as `size:32`, i.e. VARCHAR(32).

The localfs storage driver builds the opaque id as
`fileid-` + url.QueryEscape(path), so it is longer than 32 characters for any
non-trivial path, for example
`fileid-einstein%2Fshare-ci-1792481155737234595%2Fshared.txt` (59 characters).
On MySQL/MariaDB creating such a share failed with
`Error 1406 (22001): Data too long for column 'inode' at row 1`. On SQLite the
column length is not enforced, so the bug stayed hidden in the sqlite-based
unit tests and only surfaced on a MySQL deployment.

The columns are now VARCHAR(255), which holds the localfs opaque id and stays
within the InnoDB index limit for the `u_share` and `u_label` unique indexes.
