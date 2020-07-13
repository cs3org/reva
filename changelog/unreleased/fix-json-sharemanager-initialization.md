Bugfix: Fix initialization of json share manager

When an empty shares.json file existed the json share manager would fail while
trying to unmarshal the empty file.

https://github.com/cs3org/reva/issues/941
https://github.com/cs3org/reva/pull/940
