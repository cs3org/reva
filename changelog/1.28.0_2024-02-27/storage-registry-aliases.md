Bugfix: Dynamic storage registry storage_id aliases

Fixes the bug where the dynamic storage registry would not be able to
resolve storage ids like `eoshome-a`, as those are aliased and need to
be resolved into the proper storage-id (`eoshome-i01`).

https://github.com/cs3org/reva/pull/4307
