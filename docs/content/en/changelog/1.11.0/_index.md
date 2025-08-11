
---
title: "v1.11.0"
linkTitle: "v1.11.0"
weight: 40
description: >
  Changelog for Reva v1.11.0 (2021-08-03)
---

Changelog for reva 1.11.0 (2021-08-03)
=======================================

The following sections list the changes in reva 1.11.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #1899: Fix chunked uploads for new versions
 * Fix #1906: Fix copy over existing resource
 * Fix #1891: Delete Shared Resources as Receiver
 * Fix #1907: Error when creating folder with existing name
 * Fix #1937: Do not overwrite more specific matches when finding storage providers
 * Fix #1939: Fix the share jail permissions in the decomposedfs
 * Fix #1932: Numerous fixes to the owncloudsql storage driver
 * Fix #1912: Fix response when listing versions of another user
 * Fix #1910: Get user groups recursively in the cbox rest user driver
 * Fix #1904: Set Content-Length to 0 when swallowing body in the datagateway
 * Fix #1911: Fix version order in propfind responses
 * Fix #1926: Trash Bin in oCIS Storage Operations
 * Fix #1901: Fix response code when folder doesnt exist on upload
 * Enh #1785: Extend app registry with AddProvider method and mimetype filters
 * Enh #1938: Add methods to get and put context values
 * Enh #1798: Add support for a deny-all permission on references
 * Enh #1916: Generate updated protobuf bindings for EOS GRPC
 * Enh #1887: Add "a" and "l" filter for grappa queries
 * Enh #1919: Run gofmt before building
 * Enh #1927: Implement RollbackToVersion for eosgrpc (needs a newer EOS MGM)
 * Enh #1944: Implement listing supported mime types in app registry
 * Enh #1870: Be defensive about wrongly quoted etags
 * Enh #1940: Reduce memory usage when uploading with S3ng storage
 * Enh #1888: Refactoring of the webdav code
 * Enh #1900: Check for illegal names while uploading or moving files
 * Enh #1925: Refactor listing and statting across providers for virtual views

Details
-------

 * Bugfix #1899: Fix chunked uploads for new versions

   Chunked uploads didn't create a new version, when the file to upload already existed.

   https://github.com/cs3org/reva/pull/1899

 * Bugfix #1906: Fix copy over existing resource

   When the target of a copy already exists, the existing resource will be moved to the trashbin
   before executing the copy.

   https://github.com/cs3org/reva/pull/1906

 * Bugfix #1891: Delete Shared Resources as Receiver

   It is now possible to delete a shared resource as a receiver and not having the data ending up in
   the receiver's trash bin, causing a possible leak.

   https://github.com/cs3org/reva/pull/1891

 * Bugfix #1907: Error when creating folder with existing name

   When a user tried to create a folder with the name of an existing file or folder the service didn't
   return a response body containing the error.

   https://github.com/cs3org/reva/pull/1907

 * Bugfix #1937: Do not overwrite more specific matches when finding storage providers

   Depending on the order of rules in the registry it could happend that more specific matches
   (e.g. /home/Shares) were overwritten by more general ones (e.g. /home). This PR makes sure
   that the registry always returns the most specific match.

   https://github.com/cs3org/reva/pull/1937

 * Bugfix #1939: Fix the share jail permissions in the decomposedfs

   The share jail should be not writable

   https://github.com/cs3org/reva/pull/1939

 * Bugfix #1932: Numerous fixes to the owncloudsql storage driver

   The owncloudsql storage driver received numerous bugfixes and cleanups.

   https://github.com/cs3org/reva/pull/1932

 * Bugfix #1912: Fix response when listing versions of another user

   The OCS API returned the wrong response when a user tried to list the versions of another user's
   file.

   https://github.com/cs3org/reva/pull/1912

 * Bugfix #1910: Get user groups recursively in the cbox rest user driver

   https://github.com/cs3org/reva/pull/1910

 * Bugfix #1904: Set Content-Length to 0 when swallowing body in the datagateway

   When swallowing the body the Content-Lenght needs to be set to 0 to prevent proxies from reading
   the body.

   https://github.com/cs3org/reva/pull/1904

 * Bugfix #1911: Fix version order in propfind responses

   The order of the file versions in propfind responses was incorrect.

   https://github.com/cs3org/reva/pull/1911

 * Bugfix #1926: Trash Bin in oCIS Storage Operations

   Support for restoring a target folder nested deep inside the trash bin in oCIS storage. The use
   case is:

   ```console curl 'https://localhost:9200/remote.php/dav/trash-bin/einstein/f1/f2' -X
   MOVE -H 'Destination:
   https://localhost:9200/remote.php/dav/files/einstein/destination' ```

   The previous command creates the `destination` folder and moves the contents of
   `/trash-bin/einstein/f1/f2` onto it.

   Retro-compatibility in the response code with ownCloud 10. Restoring a collection to a
   non-existent nested target is not supported and MUST return `409`. The use case is:

   ```console curl 'https://localhost:9200/remote.php/dav/trash-bin/einstein/f1/f2' -X
   MOVE -H 'Destination:
   https://localhost:9200/remote.php/dav/files/einstein/this/does/not/exist' ```

   The previous command used to return `404` instead of the expected `409` by the clients.

   https://github.com/cs3org/reva/pull/1926

 * Bugfix #1901: Fix response code when folder doesnt exist on upload

   When a new file was uploaded to a non existent folder the response code was incorrect.

   https://github.com/cs3org/reva/pull/1901

 * Enhancement #1785: Extend app registry with AddProvider method and mimetype filters

   https://github.com/cs3org/reva/issues/1779
   https://github.com/cs3org/reva/pull/1785
   https://github.com/cs3org/cs3apis/pull/131

 * Enhancement #1938: Add methods to get and put context values

   Added `GetKeyValues` and `PutKeyValues` methods to fetch/put values from/to context.

   https://github.com/cs3org/reva/pull/1938

 * Enhancement #1798: Add support for a deny-all permission on references

   And implement it on the EOS storage

   http://github.com/cs3org/reva/pull/1798

 * Enhancement #1916: Generate updated protobuf bindings for EOS GRPC

   https://github.com/cs3org/reva/pull/1916

 * Enhancement #1887: Add "a" and "l" filter for grappa queries

   This PR adds the namespace filters "a" and "l" for grappa queries. With no filter will look into
   primary and e-groups, with "a" will look into primary/secondary/service/e-groups and with
   "l" will look into lightweight accounts.

   https://github.com/cs3org/reva/issues/1773
   https://github.com/cs3org/reva/pull/1887

 * Enhancement #1919: Run gofmt before building

   https://github.com/cs3org/reva/pull/1919

 * Enhancement #1927: Implement RollbackToVersion for eosgrpc (needs a newer EOS MGM)

   https://github.com/cs3org/reva/pull/1927

 * Enhancement #1944: Implement listing supported mime types in app registry

   https://github.com/cs3org/reva/pull/1944

 * Enhancement #1870: Be defensive about wrongly quoted etags

   When ocdav renders etags it will now try to correct them to the definition as *quoted strings*
   which do not contain `"`. This prevents double or triple quoted etags on the webdav api.

   https://github.com/cs3org/reva/pull/1870

 * Enhancement #1940: Reduce memory usage when uploading with S3ng storage

   The memory usage could be high when uploading files using the S3ng storage. By providing the
   actual file size when triggering `PutObject`, the overall memory usage is reduced.

   https://github.com/cs3org/reva/pull/1940

 * Enhancement #1888: Refactoring of the webdav code

   Refactored the webdav code to make it reusable.

   https://github.com/cs3org/reva/pull/1888

 * Enhancement #1900: Check for illegal names while uploading or moving files

   The code was not checking for invalid file names during uploads and moves.

   https://github.com/cs3org/reva/pull/1900

 * Enhancement #1925: Refactor listing and statting across providers for virtual views

   https://github.com/cs3org/reva/pull/1925


