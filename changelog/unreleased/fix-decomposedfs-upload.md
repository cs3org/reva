Bugfix: fix decomposedfs upload

The FS.Upload() implementation needs to handle direct uploads that did not initiate a unpload.

https://github.com/cs3org/reva/pull/2330