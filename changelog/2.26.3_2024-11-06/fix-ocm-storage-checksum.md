Bugfix: Add checksum to OCM storageprovider responses

When the remote instance of the OCM storage provider returns file checksums in
its PROPFIND responses we're now passing them through to in Stat responses.
This allows e.g. the oCIS thumbnailer to work with ocm shares.

https://github.com/cs3org/reva/pull/4908
https://github.com/owncloud/ocis/issues/10272
