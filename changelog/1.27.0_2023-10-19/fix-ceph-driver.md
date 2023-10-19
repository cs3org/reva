Enhancement: Multiple fixes for Ceph driver

* Avoid usage/creation of user homes when they are disabled in the config
* Simplify the regular uploads (not chunked)
* Avoid creation of shadow folders at the root if they are already there
* Clean up the chunked upload
* Fix panic on shutdown

https://github.com/cs3org/reva/pull/4200
