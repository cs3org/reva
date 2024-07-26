 ## Scenarios from OCIS API tests that are expected to fail with OCIS storage

 The expected failures in this file are from features in the owncloud/ocis repo.

### File
Basic file management like up and download, move, copy, properties, quota, trash, versions and chunking.

#### [invalid webdav responses for unauthorized requests.](https://github.com/owncloud/product/issues/273)
These tests succeed when running against ocis because there we handle the relevant authentication in the proxy.
-   [coreApiTrashbin/trashbinFilesFolders.feature:240](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinFilesFolders.feature#L240)
-   [coreApiTrashbin/trashbinFilesFolders.feature:241](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinFilesFolders.feature#L241)
-   [coreApiTrashbin/trashbinFilesFolders.feature:255](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinFilesFolders.feature#L255)
-   [coreApiTrashbin/trashbinFilesFolders.feature:256](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinFilesFolders.feature#L256)

#### [Custom dav properties with namespaces are rendered incorrectly](https://github.com/owncloud/ocis/issues/2140)
_ocdav: double check the webdav property parsing when custom namespaces are used_
-   [coreApiWebdavProperties/setFileProperties.feature:32](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties/setFileProperties.feature#L32)
-   [coreApiWebdavProperties/setFileProperties.feature:33](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties/setFileProperties.feature#L33)
-   [coreApiWebdavProperties/setFileProperties.feature:34](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties/setFileProperties.feature#L34)
-   [coreApiWebdavProperties/setFileProperties.feature:67](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties/setFileProperties.feature#L67)
-   [coreApiWebdavProperties/setFileProperties.feature:68](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties/setFileProperties.feature#L68)
-   [coreApiWebdavProperties/setFileProperties.feature:69](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties/setFileProperties.feature#L69)

#### [Cannot set custom webDav properties](https://github.com/owncloud/product/issues/264)
-   [coreApiWebdavProperties/getFileProperties.feature:348](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties/getFileProperties.feature#L348)
-   [coreApiWebdavProperties/getFileProperties.feature:349](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties/getFileProperties.feature#L349)
-   [coreApiWebdavProperties/getFileProperties.feature:277](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties/getFileProperties.feature#L277)
-   [coreApiWebdavProperties/getFileProperties.feature:278](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties/getFileProperties.feature#L278)
-   [coreApiWebdavProperties/getFileProperties.feature:279](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties/getFileProperties.feature#L279)
-   [coreApiWebdavProperties/getFileProperties.feature:316](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties/getFileProperties.feature#L316)

### Sync
Synchronization features like etag propagation, setting mtime and locking files

#### [Uploading an old method chunked file with checksum should fail using new DAV path](https://github.com/owncloud/ocis/issues/2323)
-   [coreApiMain/checksums.feature:219](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiMain/checksums.feature#L219)
-   [coreApiMain/checksums.feature:220](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiMain/checksums.feature#L220)

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


#### [oc:privatelink property not returned in webdav responses](https://github.com/owncloud/product/issues/262)
-   [coreApiWebdavProperties/getFileProperties.feature:317](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties/getFileProperties.feature#L317)
-   [coreApiWebdavProperties/getFileProperties.feature:318](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties/getFileProperties.feature#L318)
-   [coreApiWebdavProperties/getFileProperties.feature:350](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties/getFileProperties.feature#L350)

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
-   [coreApiWebdavOperations/search.feature:44](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L44)
-   [coreApiWebdavOperations/search.feature:60](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L60)
-   [coreApiWebdavOperations/search.feature:61](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L61)
-   [coreApiWebdavOperations/search.feature:62](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L62)
-   [coreApiWebdavOperations/search.feature:79](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L79)
-   [coreApiWebdavOperations/search.feature:80](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L80)
-   [coreApiWebdavOperations/search.feature:81](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L81)
-   [coreApiWebdavOperations/search.feature:90](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L90)
-   [coreApiWebdavOperations/search.feature:91](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L91)
-   [coreApiWebdavOperations/search.feature:92](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L92)
-   [coreApiWebdavOperations/search.feature:110](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L110)
-   [coreApiWebdavOperations/search.feature:111](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L111)
-   [coreApiWebdavOperations/search.feature:112](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L112)
-   [coreApiWebdavOperations/search.feature:130](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L130)
-   [coreApiWebdavOperations/search.feature:131](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L131)
-   [coreApiWebdavOperations/search.feature:132](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L132)
-   [coreApiWebdavOperations/search.feature:151](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L151)
-   [coreApiWebdavOperations/search.feature:152](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L152)
-   [coreApiWebdavOperations/search.feature:153](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L153)
-   [coreApiWebdavOperations/search.feature:180](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L180)
-   [coreApiWebdavOperations/search.feature:181](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L181)
-   [coreApiWebdavOperations/search.feature:182](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L182)
-   [coreApiWebdavOperations/search.feature:208](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L208)
-   [coreApiWebdavOperations/search.feature:209](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L209)
-   [coreApiWebdavOperations/search.feature:210](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L210)
-   [coreApiWebdavOperations/search.feature:229](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L229)
-   [coreApiWebdavOperations/search.feature:230](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L230)
-   [coreApiWebdavOperations/search.feature:231](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L231)

#### [Support for favorites](https://github.com/owncloud/ocis/issues/1228)
-   [coreApiFavorites/favorites.feature:101](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favorites.feature#L101)
-   [coreApiFavorites/favorites.feature:102](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favorites.feature#L102)
-   [coreApiFavorites/favorites.feature:103](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favorites.feature#L103)
-   [coreApiFavorites/favorites.feature:124](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favorites.feature#L124)
-   [coreApiFavorites/favorites.feature:125](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favorites.feature#L125)
-   [coreApiFavorites/favorites.feature:126](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favorites.feature#L126)
-   [coreApiFavorites/favorites.feature:146](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favorites.feature#L146)
-   [coreApiFavorites/favorites.feature:147](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favorites.feature#L147)
-   [coreApiFavorites/favorites.feature:174](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favorites.feature#L174)
-   [coreApiFavorites/favorites.feature:176](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favorites.feature#L176)
-   [coreApiFavorites/favorites.feature:189](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favorites.feature#L189)
-   [coreApiFavorites/favorites.feature:190](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favorites.feature#L190)
-   [coreApiFavorites/favorites.feature:191](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favorites.feature#L191)

#### [WWW-Authenticate header for unauthenticated requests is not clear](https://github.com/owncloud/ocis/issues/2285)
-   [coreApiWebdavOperations/refuseAccess.feature:21](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/refuseAccess.feature#L21)
-   [coreApiWebdavOperations/refuseAccess.feature:22](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/refuseAccess.feature#L22)
-   [coreApiWebdavOperations/refuseAccess.feature:34](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/refuseAccess.feature#L34)
-   [coreApiWebdavOperations/refuseAccess.feature:35](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/refuseAccess.feature#L35)
-   [coreApiWebdavOperations/refuseAccess.feature:36](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/refuseAccess.feature#L36)



### Won't fix
Not everything needs to be implemented for ocis. While the oc10 testsuite covers these things we are not looking at them right now.

* _The `OC-LazyOps` header is [no longer supported by the client](https://github.com/owncloud/client/pull/8398), implmenting this is not necessary for a first production release. We plan to have an uploed state machine to visualize the state of a file, see https://github.com/owncloud/ocis/issues/214_
* _Blacklisted ignored files are no longer required because ocis can handle `.htaccess` files without security implications introduced by serving user provided files with apache._

#### [Blacklist files extensions](https://github.com/owncloud/ocis/issues/2177)
-   [coreApiWebdavProperties/copyFile.feature:108](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties/copyFile.feature#L108)
-   [coreApiWebdavProperties/copyFile.feature:109](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties/copyFile.feature#L109)
-   [coreApiWebdavProperties/copyFile.feature:107](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties/copyFile.feature#L107)
-   [coreApiWebdavProperties/createFileFolder.feature:95](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties/createFileFolder.feature#L95)
-   [coreApiWebdavProperties/createFileFolder.feature:96](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties/createFileFolder.feature#L96)
-   [coreApiWebdavProperties/createFileFolder.feature:97](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties/createFileFolder.feature#L97)
-   [coreApiWebdavUpload/uploadFile.feature:152](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUpload/uploadFile.feature#L152)
-   [coreApiWebdavUpload/uploadFile.feature:153](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUpload/uploadFile.feature#L153)
-   [coreApiWebdavUpload/uploadFile.feature:154](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUpload/uploadFile.feature#L154)
-   [coreApiWebdavUpload/uploadFile.feature:379](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUpload/uploadFile.feature#L379)
-   [coreApiWebdavMove2/moveFile.feature:179](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove2/moveFile.feature#L179)
-   [coreApiWebdavMove2/moveFile.feature:100](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove2/moveFile.feature#L100)
-   [coreApiWebdavMove2/moveFile.feature:101](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove2/moveFile.feature#L101)

### To triage
_The below features have been added after I last categorized them. AFAICT they are bugs. @jfd_

#### [PATCH request for TUS upload with wrong checksum gives incorrect response](https://github.com/owncloud/ocis/issues/1755)
-   [coreApiWebdavUploadTUS/checksums.feature:74](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L74)
-   [coreApiWebdavUploadTUS/checksums.feature:75](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L75)
-   [coreApiWebdavUploadTUS/checksums.feature:76](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L76)
-   [coreApiWebdavUploadTUS/checksums.feature:77](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L77)
-   [coreApiWebdavUploadTUS/checksums.feature:78](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L78)
-   [coreApiWebdavUploadTUS/checksums.feature:79](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L79)
-   [coreApiWebdavUploadTUS/checksums.feature:147](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L147)
-   [coreApiWebdavUploadTUS/checksums.feature:148](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L148)
-   [coreApiWebdavUploadTUS/checksums.feature:149](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L149)
-   [coreApiWebdavUploadTUS/checksums.feature:192](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L192)
-   [coreApiWebdavUploadTUS/checksums.feature:193](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L193)
-   [coreApiWebdavUploadTUS/checksums.feature:194](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L194)
-   [coreApiWebdavUploadTUS/checksums.feature:195](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L195)
-   [coreApiWebdavUploadTUS/checksums.feature:196](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L196)
-   [coreApiWebdavUploadTUS/checksums.feature:197](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L197)
-   [coreApiWebdavUploadTUS/checksums.feature:240](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L240)
-   [coreApiWebdavUploadTUS/checksums.feature:241](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L241)
-   [coreApiWebdavUploadTUS/checksums.feature:242](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L242)
-   [coreApiWebdavUploadTUS/checksums.feature:243](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L243)
-   [coreApiWebdavUploadTUS/checksums.feature:244](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L244)
-   [coreApiWebdavUploadTUS/checksums.feature:245](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L245)
-   [coreApiWebdavUploadTUS/optionsRequest.feature:10](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/optionsRequest.feature#L10)
-   [coreApiWebdavUploadTUS/optionsRequest.feature:25](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/optionsRequest.feature#L25)

#### [TUS OPTIONS requests do not reply with TUS headers when invalid password](https://github.com/owncloud/ocis/issues/1012)
-   [coreApiWebdavUploadTUS/optionsRequest.feature:40](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/optionsRequest.feature#L40)
-   [coreApiWebdavUploadTUS/optionsRequest.feature:55](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/optionsRequest.feature#L55)

### [send PUT requests to another user's webDav endpoints as normal user](https://github.com/owncloud/ocis/issues/2893)
-   [coreApiAuth/webDavPUTAuth.feature:46](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuth/webDavPUTAuth.feature#L46)
-   [coreApiAuth/webDavPUTAuth.feature:58](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuth/webDavPUTAuth.feature#L58)

#### [Renaming resource to banned name is allowed in spaces webdav](https://github.com/owncloud/ocis/issues/3099)
-   [coreApiWebdavMove1/moveFolder.feature:36](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove1/moveFolder.feature#L36)
-   [coreApiWebdavMove1/moveFolder.feature:50](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove1/moveFolder.feature#L50)
- [coreApiWebdavMove1/moveFolder.feature:64](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove1/moveFolder.feature#L64)
-   [coreApiWebdavMove2/moveFile.feature:102](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove2/moveFile.feature#L102)

#### [REPORT method on spaces returns an incorrect d:href response](https://github.com/owncloud/ocis/issues/3111)
-   [coreApiFavorites/favorites.feature:145](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favorites.feature#L145)
-   [coreApiFavorites/favorites.feature:175](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favorites.feature#L175)

#### [Cannot disable the dav propfind depth infinity for resources](https://github.com/owncloud/ocis/issues/3720)
-   [coreApiWebdavOperations/propfind.feature:31](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/propfind.feature#L31)

#### [HTTP status code differ while deleting file of another user's trash bin](https://github.com/owncloud/ocis/issues/3544)

-   [coreApiTrashbin/trashbinDelete.feature:92](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinDelete.feature#L92)

### [MOVE a file into same folder with same name returns 404 instead of 403](https://github.com/owncloud/ocis/issues/1976)

- [coreApiWebdavMove2/moveFile.feature:143](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove2/moveFile.feature#L143)
- [coreApiWebdavMove2/moveFile.feature:177](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove2/moveFile.feature#L177)
- [coreApiWebdavMove2/moveFile.feature:178](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove2/moveFile.feature#L178)
- [coreApiWebdavMove1/moveFolder.feature:217](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove1/moveFolder.feature#L217)
- [coreApiWebdavMove1/moveFolder.feature:218](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove1/moveFolder.feature#L218)
- [coreApiWebdavMove1/moveFolder.feature:219](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove1/moveFolder.feature#L219)

- Note: always have an empty line at the end of this file.
The bash script that processes this file may not process a scenario reference on the last line.
