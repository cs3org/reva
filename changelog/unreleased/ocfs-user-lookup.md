Bugfix: ocfs: Lookup user to render template properly

Currently, the username is used to construct paths, which breaks when mounting the `owncloud` storage driver at `/oc` and then expecting paths that use the username like `/oc/einstein/foo` to work, because they will mismatch the path that is used from propagation which uses `/oc/u-u-i-d` as the root, giving an `internal path outside root` error

https://github.com/cs3org/reva/pull/1052
