## Scenarios from OCIS API tests that are expected to fail with OCIS storage

The expected failures in this file are from features in the owncloud/ocis repo.

### File
Basic file management like up and download, move, copy, properties, quota, trash, versions and chunking.

#### [invalid webdav responses for unauthorized requests.](https://github.com/owncloud/product/issues/273)
-   [coreApiTrashbin/trashbinFilesFolders.feature:277](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinFilesFolders.feature#L277)
-   [coreApiTrashbin/trashbinFilesFolders.feature:282](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinFilesFolders.feature#L282)
-   [coreApiTrashbin/trashbinFilesFolders.feature:296](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinFilesFolders.feature#L296)
-   [coreApiTrashbin/trashbinFilesFolders.feature:301](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinFilesFolders.feature#L301)

#### [Custom dav properties with namespaces are rendered incorrectly](https://github.com/owncloud/ocis/issues/2140)
_ocdav: double check the webdav property parsing when custom namespaces are used_
-   [coreApiWebdavProperties/setFileProperties.feature:36](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties/setFileProperties.feature#L36)
-   [coreApiWebdavProperties/setFileProperties.feature:37](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties/setFileProperties.feature#L37)
-   [coreApiWebdavProperties/setFileProperties.feature:42](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties/setFileProperties.feature#L42)
-   [coreApiWebdavProperties/setFileProperties.feature:79](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties/setFileProperties.feature#L79)
-   [coreApiWebdavProperties/setFileProperties.feature:78](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties/setFileProperties.feature#L78)
-   [coreApiWebdavProperties/setFileProperties.feature:84](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties/setFileProperties.feature#L84)

#### [Cannot set custom webDav properties](https://github.com/owncloud/product/issues/264)
-   [coreApiWebdavProperties/getFileProperties.feature:348](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties/getFileProperties.feature#L348)
-   [coreApiWebdavProperties/getFileProperties.feature:349](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties/getFileProperties.feature#L349)
-   [coreApiWebdavProperties/getFileProperties.feature:354](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties/getFileProperties.feature#L354)
-   [coreApiWebdavProperties/getFileProperties.feature:384](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties/getFileProperties.feature#L384)
-   [coreApiWebdavProperties/getFileProperties.feature:385](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties/getFileProperties.feature#L385)
-   [coreApiWebdavProperties/getFileProperties.feature:390](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties/getFileProperties.feature#L390)

### Sync
Synchronization features like etag propagation, setting mtime and locking files

#### [Uploading an old method chunked file with checksum should fail using new DAV path](https://github.com/owncloud/ocis/issues/2323)
-   [coreApiMain/checksums.feature:269](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiMain/checksums.feature#L269)
-   [coreApiMain/checksums.feature:274](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiMain/checksums.feature#L274)

#### [Webdav LOCK operations](https://github.com/owncloud/ocis/issues/1284)
-   [coreApiWebdavLocks2/independentLocks.feature:25](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks2/independentLocks.feature#L25)
-   [coreApiWebdavLocks2/independentLocks.feature:26](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks2/independentLocks.feature#L26)
-   [coreApiWebdavLocks2/independentLocks.feature:27](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks2/independentLocks.feature#L27)
-   [coreApiWebdavLocks2/independentLocks.feature:28](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks2/independentLocks.feature#L28)
-   [coreApiWebdavLocks2/independentLocks.feature:33](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks2/independentLocks.feature#L33)
-   [coreApiWebdavLocks2/independentLocks.feature:34](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks2/independentLocks.feature#L34)
-   [coreApiWebdavLocks2/independentLocks.feature:53](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks2/independentLocks.feature#L53)
-   [coreApiWebdavLocks2/independentLocks.feature:54](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks2/independentLocks.feature#L54)
-   [coreApiWebdavLocks2/independentLocks.feature:55](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks2/independentLocks.feature#L55)
-   [coreApiWebdavLocks2/independentLocks.feature:56](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks2/independentLocks.feature#L56)
-   [coreApiWebdavLocks2/independentLocks.feature:57](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks2/independentLocks.feature#L57)
-   [coreApiWebdavLocks2/independentLocks.feature:58](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks2/independentLocks.feature#L58)
-   [coreApiWebdavLocks2/independentLocks.feature:59](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks2/independentLocks.feature#L59)
-   [coreApiWebdavLocks2/independentLocks.feature:60](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks2/independentLocks.feature#L60)
-   [coreApiWebdavLocks2/independentLocks.feature:65](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks2/independentLocks.feature#L65)
-   [coreApiWebdavLocks2/independentLocks.feature:66](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks2/independentLocks.feature#L66)
-   [coreApiWebdavLocks2/independentLocks.feature:67](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks2/independentLocks.feature#L67)
-   [coreApiWebdavLocks2/independentLocks.feature:68](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks2/independentLocks.feature#L68)
-   [coreApiWebdavLocksUnlock/unlock.feature:23](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlock.feature#L23)
-   [coreApiWebdavLocksUnlock/unlock.feature:24](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlock.feature#L24)
-   [coreApiWebdavLocksUnlock/unlock.feature:29](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlock.feature#L29)
-   [coreApiWebdavLocksUnlock/unlock.feature:43](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlock.feature#L43)
-   [coreApiWebdavLocksUnlock/unlock.feature:44](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlock.feature#L44)
-   [coreApiWebdavLocksUnlock/unlock.feature:67](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlock.feature#L67)
-   [coreApiWebdavLocksUnlock/unlock.feature:68](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlock.feature#L68)
-   [coreApiWebdavLocksUnlock/unlock.feature:69](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlock.feature#L69)
-   [coreApiWebdavLocksUnlock/unlock.feature:66](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlock.feature#L66)
-   [coreApiWebdavLocksUnlock/unlock.feature:74](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlock.feature#L74)
-   [coreApiWebdavLocksUnlock/unlock.feature:75](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlock.feature#L75)


#### [Set quota over settings](https://github.com/owncloud/ocis/issues/1290)
_requires a [CS3 user provisioning api that can update the quota for a user](https://github.com/cs3org/cs3apis/pull/95#issuecomment-772780683)_
- [coreApiSharePublicLink3/uploadToPublicLinkShare.feature:91](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharePublicLink3/uploadToPublicLinkShare.feature#L91)
- [coreApiSharePublicLink3/uploadToPublicLinkShare.feature:101](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharePublicLink3/uploadToPublicLinkShare.feature#L101)

#### [oc:privatelink property not returned in webdav responses](https://github.com/owncloud/product/issues/262)
-   [coreApiWebdavProperties/getFileProperties.feature:301](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties/getFileProperties.feature#L301)
-   [coreApiWebdavProperties/getFileProperties.feature:302](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties/getFileProperties.feature#L302)
-   [coreApiWebdavProperties/getFileProperties.feature:307](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties/getFileProperties.feature#L307)

### User Management
User and group management features

### Other
API, search, favorites, config, capabilities, not existing endpoints, CORS and others

#### [no robots.txt available](https://github.com/owncloud/ocis/issues/1314)
-   [coreApiMain/main.feature:7](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiMain/main.feature#L7) Scenario: robots.txt file should be accessible

#### [Ability to return error messages in Webdav response bodies](https://github.com/owncloud/ocis/issues/1293)
-   [coreApiAuth/ocsGETAuth.feature:99](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuth/ocsGETAuth.feature#L99) Scenario:using OCS with admin basic auth

#### [sending MKCOL requests to another or non-existing user's webDav endpoints as normal user should return 404](https://github.com/owncloud/ocis/issues/5049)
_ocdav: api compatibility, return correct status code_
-   [coreApiAuth/webDavMKCOLAuth.feature:42](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuth/webDavMKCOLAuth.feature#L42) Scenario: send MKCOL requests to another user's webDav endpoints as normal user
-   [coreApiAuth/webDavMKCOLAuth.feature:53](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuth/webDavMKCOLAuth.feature#L53) Scenario: send MKCOL requests to another user's webDav endpoints as normal user using the spaces WebDAV API

#### [trying to lock file of another user gives http 200](https://github.com/owncloud/ocis/issues/2176)
-   [coreApiAuth/webDavLOCKAuth.feature:46](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuth/webDavLOCKAuth.feature#L46) Scenario: send LOCK requests to another user's webDav endpoints as normal user
-   [coreApiAuth/webDavLOCKAuth.feature:58](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuth/webDavLOCKAuth.feature#L58) Scenario: send LOCK requests to another user's webDav endpoints as normal user using the spaces WebDAV API

#### [send (MOVE, COPY) requests to another user's webDav endpoints as normal user gives 400 instead of 403](https://github.com/owncloud/ocis/issues/3882)
_ocdav: api compatibility, return correct status code_
-   [coreApiAuth/webDavMOVEAuth.feature:55](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuth/webDavMOVEAuth.feature#L55) Scenario: send MOVE requests to another user's webDav endpoints as normal user using the spaces WebDAV API
-   [coreApiAuth/webDavCOPYAuth.feature:55](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuth/webDavCOPYAuth.feature#L55)

#### [send POST requests to another user's webDav endpoints as normal user](https://github.com/owncloud/ocis/issues/1287)
_ocdav: api compatibility, return correct status code_
-   [coreApiAuth/webDavPOSTAuth.feature:46](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuth/webDavPOSTAuth.feature#L46)  Scenario: send POST requests to another user's webDav endpoints as normal user
-   [coreApiAuth/webDavPOSTAuth.feature:55](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuth/webDavPOSTAuth.feature#L55)  Scenario: send POST requests to another user's webDav endpoints as normal user using the spaces WebDAV API

#### [Using double slash in URL to access a folder gives 501 and other status codes](https://github.com/owncloud/ocis/issues/1667)
-   [coreApiAuth/webDavSpecialURLs.feature:123](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuth/webDavSpecialURLs.feature#L123)
-   [coreApiAuth/webDavSpecialURLs.feature:134](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuth/webDavSpecialURLs.feature#L134)
-   [coreApiAuth/webDavSpecialURLs.feature:165](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuth/webDavSpecialURLs.feature#L165)
-   [coreApiAuth/webDavSpecialURLs.feature:176](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuth/webDavSpecialURLs.feature#L176)

#### [Difference in response content of status.php and default capabilities](https://github.com/owncloud/ocis/issues/1286)
-   [coreApiCapabilities/capabilitiesWithNormalUser.feature:13](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiCapabilities/capabilitiesWithNormalUser.feature#L13) Scenario: getting default capabilities with normal user

#### [spaces endpoint does not allow REPORT requests](https://github.com/owncloud/ocis/issues/4034)
-   [coreApiWebdavOperations/search.feature:42](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L42)
-   [coreApiWebdavOperations/search.feature:43](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L43)
-   [coreApiWebdavOperations/search.feature:48](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L48)
-   [coreApiWebdavOperations/search.feature:64](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L64)
-   [coreApiWebdavOperations/search.feature:65](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L65)
-   [coreApiWebdavOperations/search.feature:70](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L70)
-   [coreApiWebdavOperations/search.feature:87](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L87)
-   [coreApiWebdavOperations/search.feature:88](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L88)
-   [coreApiWebdavOperations/search.feature:93](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L93)
-   [coreApiWebdavOperations/search.feature:102](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L102)
-   [coreApiWebdavOperations/search.feature:103](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L103)
-   [coreApiWebdavOperations/search.feature:126](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L126)
-   [coreApiWebdavOperations/search.feature:127](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L127)
-   [coreApiWebdavOperations/search.feature:108](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L108)
-   [coreApiWebdavOperations/search.feature:132](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L132)
-   [coreApiWebdavOperations/search.feature:150](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L150)
-   [coreApiWebdavOperations/search.feature:151](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L151)
-   [coreApiWebdavOperations/search.feature:156](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L156)
-   [coreApiWebdavOperations/search.feature:176](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L176)
-   [coreApiWebdavOperations/search.feature:175](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L175)
-   [coreApiWebdavOperations/search.feature:181](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L181)
-   [coreApiWebdavOperations/search.feature:209](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L209)
-   [coreApiWebdavOperations/search.feature:208](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L208)
-   [coreApiWebdavOperations/search.feature:214](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L214)
-   [coreApiWebdavOperations/search.feature:241](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L241)
-   [coreApiWebdavOperations/search.feature:240](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L240)
-   [coreApiWebdavOperations/search.feature:246](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L246)
-   [coreApiWebdavOperations/search.feature:266](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L266)
-   [coreApiWebdavOperations/search.feature:265](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L265)
-   [coreApiWebdavOperations/search.feature:271](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L271)

#### [Support for favorites](https://github.com/owncloud/ocis/issues/1228)
-   [coreApiFavorites/favorites.feature:169](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favorites.feature#L169)
-   [coreApiFavorites/favorites.feature:170](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favorites.feature#L170)
-   [coreApiFavorites/favorites.feature:202](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favorites.feature#L202)
-   [coreApiFavorites/favorites.feature:203](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favorites.feature#L203)
-   [coreApiFavorites/favorites.feature:175](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favorites.feature#L175)
-   [coreApiFavorites/favorites.feature:144](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favorites.feature#L144)
-   [coreApiFavorites/favorites.feature:145](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favorites.feature#L145)
-   [coreApiFavorites/favorites.feature:208](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favorites.feature#L208)

#### [WWW-Authenticate header for unauthenticated requests is not clear](https://github.com/owncloud/ocis/issues/2285)
-   [coreApiWebdavOperations/refuseAccess.feature:21](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/refuseAccess.feature#L21)
-   [coreApiWebdavOperations/refuseAccess.feature:22](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/refuseAccess.feature#L22)
-   [coreApiWebdavOperations/refuseAccess.feature:34](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/refuseAccess.feature#L34)
-   [coreApiWebdavOperations/refuseAccess.feature:35](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/refuseAccess.feature#L35)
-   [coreApiWebdavOperations/refuseAccess.feature:40](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/refuseAccess.feature#L40)


### Won't fix
Not everything needs to be implemented for ocis. While the oc10 testsuite covers these things we are not looking at them right now.

* _The `OC-LazyOps` header is [no longer supported by the client](https://github.com/owncloud/client/pull/8398), implmenting this is not necessary for a first production release. We plan to have an uploed state machine to visualize the state of a file, see https://github.com/owncloud/ocis/issues/214_
* _Blacklisted ignored files are no longer required because ocis can handle `.htaccess` files without security implications introduced by serving user provided files with apache._

#### [Blacklist files extensions](https://github.com/owncloud/ocis/issues/2177)
-   [coreApiWebdavProperties/copyFile.feature:117](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties/copyFile.feature#L117)
-   [coreApiWebdavProperties/copyFile.feature:118](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties/copyFile.feature#L118)
-   [coreApiWebdavProperties/copyFile.feature:123](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties/copyFile.feature#L123)
-   [coreApiWebdavProperties/createFileFolder.feature:106](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties/createFileFolder.feature#L106)
-   [coreApiWebdavProperties/createFileFolder.feature:107](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties/createFileFolder.feature#L107)
-   [coreApiWebdavProperties/createFileFolder.feature:112](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties/createFileFolder.feature#L112)
-   [coreApiWebdavUpload/uploadFile.feature:180](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUpload/uploadFile.feature#L180)
-   [coreApiWebdavUpload/uploadFile.feature:181](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUpload/uploadFile.feature#L181)
-   [coreApiWebdavUpload/uploadFile.feature:186](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUpload/uploadFile.feature#L186)
-   [coreApiWebdavMove2/moveFile.feature:179](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove2/moveFile.feature#L179)
-   [coreApiWebdavMove2/moveFile.feature:217](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove2/moveFile.feature#L217)
-   [coreApiWebdavMove2/moveFile.feature:218](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove2/moveFile.feature#L218)

#### [cannot set blacklisted file names](https://github.com/owncloud/product/issues/260)
-   [coreApiWebdavMove1/moveFolderToBlacklistedName.feature:20](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove1/moveFolderToBlacklistedName.feature#L20)
-   [coreApiWebdavMove1/moveFolderToBlacklistedName.feature:21](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove1/moveFolderToBlacklistedName.feature#L21)
-   [coreApiWebdavMove1/moveFolderToBlacklistedName.feature:26](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove1/moveFolderToBlacklistedName.feature#L26)
-   [coreApiWebdavMove2/moveFileToBlacklistedName.feature:18](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove2/moveFileToBlacklistedName.feature#L18)
-   [coreApiWebdavMove2/moveFileToBlacklistedName.feature:19](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove2/moveFileToBlacklistedName.feature#L19)

### To triage
_The below features have been added after I last categorized them. AFAICT they are bugs. @jfd_

#### [PATCH request for TUS upload with wrong checksum gives incorrect response](https://github.com/owncloud/ocis/issues/1755)
-   [coreApiWebdavUploadTUS/checksums.feature:86](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L86)
-   [coreApiWebdavUploadTUS/checksums.feature:87](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L87)
-   [coreApiWebdavUploadTUS/checksums.feature:88](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L88)
-   [coreApiWebdavUploadTUS/checksums.feature:89](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L89)
-   [coreApiWebdavUploadTUS/checksums.feature:94](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L94)
-   [coreApiWebdavUploadTUS/checksums.feature:95](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L95)
-   [coreApiWebdavUploadTUS/checksums.feature:175](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L175)
-   [coreApiWebdavUploadTUS/checksums.feature:176](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L176)
-   [coreApiWebdavUploadTUS/checksums.feature:181](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L181)
-   [coreApiWebdavUploadTUS/checksums.feature:228](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L228)
-   [coreApiWebdavUploadTUS/checksums.feature:229](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L229)
-   [coreApiWebdavUploadTUS/checksums.feature:230](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L230)
-   [coreApiWebdavUploadTUS/checksums.feature:231](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L231)
-   [coreApiWebdavUploadTUS/checksums.feature:236](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L236)
-   [coreApiWebdavUploadTUS/checksums.feature:237](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L237)
-   [coreApiWebdavUploadTUS/checksums.feature:284](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L284)
-   [coreApiWebdavUploadTUS/checksums.feature:285](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L285)
-   [coreApiWebdavUploadTUS/checksums.feature:286](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L286)
-   [coreApiWebdavUploadTUS/checksums.feature:287](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L287)
-   [coreApiWebdavUploadTUS/checksums.feature:292](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L292)
-   [coreApiWebdavUploadTUS/checksums.feature:293](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L293)
-   [coreApiWebdavUploadTUS/optionsRequest.feature:10](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/optionsRequest.feature#L10)
-   [coreApiWebdavUploadTUS/optionsRequest.feature:25](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/optionsRequest.feature#L25)

#### [TUS OPTIONS requests do not reply with TUS headers when invalid password](https://github.com/owncloud/ocis/issues/1012)
-   [coreApiWebdavUploadTUS/optionsRequest.feature:40](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/optionsRequest.feature#L40)
-   [coreApiWebdavUploadTUS/optionsRequest.feature:55](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/optionsRequest.feature#L55)

### [send PUT requests to another user's webDav endpoints as normal user](https://github.com/owncloud/ocis/issues/2893)
-   [coreApiAuth/webDavPUTAuth.feature:46](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuth/webDavPUTAuth.feature#L46)
-   [coreApiAuth/webDavPUTAuth.feature:58](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuth/webDavPUTAuth.feature#L58)

#### [Renaming resource to banned name is allowed in spaces webdav](https://github.com/owncloud/ocis/issues/3099)
-   [coreApiWebdavMove1/moveFolder.feature:44](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove1/moveFolder.feature#L44)
-   [coreApiWebdavMove1/moveFolder.feature:62](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove1/moveFolder.feature#L62)
- [coreApiWebdavMove1/moveFolder.feature:80](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove1/moveFolder.feature#L80)
-   [coreApiWebdavMove2/moveFile.feature:223](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove2/moveFile.feature#L223)
-   [coreApiWebdavMove2/moveFileToBlacklistedName.feature:24](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove2/moveFileToBlacklistedName.feature#L24)

#### [REPORT method on spaces returns an incorrect d:href response](https://github.com/owncloud/ocis/issues/3111)
-   [coreApiFavorites/favorites.feature:117](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favorites.feature#L117)
-   [coreApiFavorites/favorites.feature:118](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favorites.feature#L118)
-   [coreApiFavorites/favorites.feature:123](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favorites.feature#L123)
-   [coreApiFavorites/favorites.feature:221](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favorites.feature#L221)
-   [coreApiFavorites/favorites.feature:222](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favorites.feature#L222)
-   [coreApiFavorites/favorites.feature:227](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favorites.feature#L227)
-   [coreApiFavorites/favorites.feature:150](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favorites.feature#L150)

#### [Cannot disable the dav propfind depth infinity for resources](https://github.com/owncloud/ocis/issues/3720)
-   [coreApiWebdavOperations/propfind.feature:39](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/propfind.feature#L39)

#### [HTTP status code differ while deleting file of another user's trash bin](https://github.com/owncloud/ocis/issues/3544)

-   [coreApiTrashbin/trashbinDelete.feature:105](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinDelete.feature#L105)

### [MOVE a file into same folder with same name returns 404 instead of 403](https://github.com/owncloud/ocis/issues/1976)

- [coreApiWebdavMove2/moveFile.feature:120](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove2/moveFile.feature#L120)
- [coreApiWebdavMove2/moveFile.feature:121](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove2/moveFile.feature#L121)
- [coreApiWebdavMove2/moveFile.feature:126](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove2/moveFile.feature#L126)
- [coreApiWebdavMove1/moveFolder.feature:253](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove1/moveFolder.feature#L253)
- [coreApiWebdavMove1/moveFolder.feature:254](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove1/moveFolder.feature#L254)
- [coreApiWebdavMove1/moveFolder.feature:259](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove1/moveFolder.feature#L259)

Note: always have an empty line at the end of this file.
The bash script that processes this file may not process a scenario reference on the last line.
