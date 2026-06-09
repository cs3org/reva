Bugfix: eosfs: fixed lock retrieval

getLock on a file with an empty lock now returns no lock as opposed
to failing with a malformed lock exception

https://github.com/cs3org/reva/pull/5648
