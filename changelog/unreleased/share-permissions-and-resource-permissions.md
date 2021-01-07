Enhancement: calculate and expose actual file permission set

Instead of hardcoding the permissions set for every file and folder to ListContainer:true, CreateContainer:true and always reporting the hardcoded string WCKDNVR for the WebDAV permissions we now aggregate the actual cs3 resource permission set in the storage drivers and correctly map them to ocs permissions and webdav permissions using a common role struct that encapsulates the mapping logic.

https://github.com/cs3org/reva/pull/1368
https://github.com/owncloud/ocis/issues/552
https://github.com/owncloud/ocis/issues/762
https://github.com/owncloud/ocis/issues/763
https://github.com/owncloud/ocis/issues/893
https://github.com/owncloud/ocis/issues/1126
https://github.com/owncloud/ocis-reva/issues/47
https://github.com/owncloud/ocis-reva/issues/315
https://github.com/owncloud/ocis-reva/issues/316
https://github.com/owncloud/product/issues/270