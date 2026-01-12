Enhancement: add indexes to share db for fields that may be queried

In a previous change, some indexes for fields that are queried had
been replaced by a composite index, which cannot be used for certain
queries that we do. So, this PR brings back the non-composite indexes.

https://github.com/cs3org/reva/pull/5470
