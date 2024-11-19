Changelog for reva 2.26.6 (2024-11-19)
=======================================

The following sections list the changes in reva 2.26.6 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #4955: Allow small clock skew in reva token validation
*   Fix #4929: Fix flaky posixfs integration tests
*   Fix #4953: Avoid gateway panics
*   Fix #4959: Fix missing file touched event
*   Fix #4933: Fix federated sharing when using an external identity provider
*   Fix #4935: Enable datatx log
*   Fix #4936: Do not delete mlock files
*   Fix #4954: Prevent a panic when logging an error
*   Fix #4956: Improve posixfs error handling and logging
*   Fix #4951: Pass the initialized logger down the stack

Details
-------

*   Bugfix #4955: Allow small clock skew in reva token validation

   Allow for a small clock skew (3 seconds by default) when validating reva tokens as the different
   services might be running on different machines.

   https://github.com/cs3org/reva/issues/4952
   https://github.com/cs3org/reva/pull/4955

*   Bugfix #4929: Fix flaky posixfs integration tests

   We fixed a problem with the posixfs integration tests where the in-memory id cache sometimes
   hadn't caught up with the cleanup between test runs leading to flaky failures.

   https://github.com/cs3org/reva/pull/4929

*   Bugfix #4953: Avoid gateway panics

   The gateway would panic if there is a missing user in the context. Now it errors instead.

   https://github.com/cs3org/reva/issues/4953

*   Bugfix #4959: Fix missing file touched event

   We have fixed an issue where the `file touched` event was not being triggered when an office
   document was created.

   https://github.com/owncloud/ocis/issues/8950
   https://github.com/cs3org/reva/pull/4959

*   Bugfix #4933: Fix federated sharing when using an external identity provider

   We fixes and issue that caused federated sharing to fail when the identity provider url did not
   match the federation provider url.

   https://github.com/cs3org/reva/pull/4933

*   Bugfix #4935: Enable datatx log

   We now pass a properly initialized logger to the datatx implementations, allowing the tus
   handler to log with the same level as the rest of reva.

   https://github.com/cs3org/reva/pull/4935

*   Bugfix #4936: Do not delete mlock files

   To prevent stale NFS file handles we no longer delete empty mlock files after updating the
   metadata.

   https://github.com/cs3org/reva/pull/4936
   https://github.com/cs3org/reva/pull/4924

*   Bugfix #4954: Prevent a panic when logging an error

   We fixed a panic when constructing a path failed to get the parent for a node.

   https://github.com/cs3org/reva/pull/4954

*   Bugfix #4956: Improve posixfs error handling and logging

   We improved error handling and logging in the posixfs storage driver.

   https://github.com/cs3org/reva/pull/4956

*   Bugfix #4951: Pass the initialized logger down the stack

   We now make the initialized logger available to grpc services and storage drivers, which
   allows for easier and more uniform logging.

   https://github.com/cs3org/reva/pull/4951
