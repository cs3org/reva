 ## Scenarios from OCIS API tests that are expected to fail with OCIS storage

 The expected failures in this file are from features in the owncloud/ocis repo.

### File
Basic file management like up and download, move, copy, properties, quota, trash, versions and chunking.

#### [invalid webdav responses for unauthorized requests.](https://github.com/owncloud/product/issues/273)
These tests succeed when running against ocis because there we handle the relevant authentication in the proxy.
-   [coreApiTrashbin/trashbinFilesFolders.feature:268](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinFilesFolders.feature#L268)
-   [coreApiTrashbin/trashbinFilesFolders.feature:273](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinFilesFolders.feature#L273)
-   [coreApiTrashbin/trashbinFilesFolders.feature:287](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinFilesFolders.feature#L287)
-   [coreApiTrashbin/trashbinFilesFolders.feature:292](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinFilesFolders.feature#L292)

#### [Getting information about a folder overwritten by a file gives 500 error instead of 404](https://github.com/owncloud/ocis/issues/1239)
These tests are about overwriting files or folders in the `Shares` folder of a user.
-   [coreApiWebdavProperties1/copyFile.feature:272](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/copyFile.feature#L272)
-   [coreApiWebdavProperties1/copyFile.feature:273](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/copyFile.feature#L273)
-   [coreApiWebdavProperties1/copyFile.feature:290](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/copyFile.feature#L290)
-   [coreApiWebdavProperties1/copyFile.feature:291](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/copyFile.feature#L291)

#### [Custom dav properties with namespaces are rendered incorrectly](https://github.com/owncloud/ocis/issues/2140)
_ocdav: double check the webdav property parsing when custom namespaces are used_
-   [coreApiWebdavProperties1/setFileProperties.feature:37](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/setFileProperties.feature#L37)
-   [coreApiWebdavProperties1/setFileProperties.feature:38](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/setFileProperties.feature#L38)
-   [coreApiWebdavProperties1/setFileProperties.feature:43](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/setFileProperties.feature#L43)
-   [coreApiWebdavProperties1/setFileProperties.feature:78](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/setFileProperties.feature#L78)
-   [coreApiWebdavProperties1/setFileProperties.feature:79](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/setFileProperties.feature#L79)
-   [coreApiWebdavProperties1/setFileProperties.feature:84](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/setFileProperties.feature#L84)

#### [Cannot set custom webDav properties](https://github.com/owncloud/product/issues/264)
-   [coreApiWebdavProperties2/getFileProperties.feature:341](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties2/getFileProperties.feature#L341)
-   [coreApiWebdavProperties2/getFileProperties.feature:347](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties2/getFileProperties.feature#L347)
-   [coreApiWebdavProperties2/getFileProperties.feature:342](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties2/getFileProperties.feature#L342)
-   [coreApiWebdavProperties2/getFileProperties.feature:377](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties2/getFileProperties.feature#L377)
-   [coreApiWebdavProperties2/getFileProperties.feature:383](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties2/getFileProperties.feature#L383)
-   [coreApiWebdavProperties2/getFileProperties.feature:378](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties2/getFileProperties.feature#L378)

### Sync
Synchronization features like etag propagation, setting mtime and locking files

#### [Uploading an old method chunked file with checksum should fail using new DAV path](https://github.com/owncloud/ocis/issues/2323)
-   [coreApiMain/checksums.feature:266](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiMain/checksums.feature#L266)
-   [coreApiMain/checksums.feature:261](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiMain/checksums.feature#L261)

#### [Webdav LOCK operations](https://github.com/owncloud/ocis/issues/1284)
-   [coreApiWebdavLocks/exclusiveLocks.feature:49](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks/exclusiveLocks.feature#L49)
-   [coreApiWebdavLocks/exclusiveLocks.feature:50](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks/exclusiveLocks.feature#L50)
-   [coreApiWebdavLocks/exclusiveLocks.feature:51](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks/exclusiveLocks.feature#L51)
-   [coreApiWebdavLocks/exclusiveLocks.feature:52](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks/exclusiveLocks.feature#L52)
-   [coreApiWebdavLocks/exclusiveLocks.feature:57](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks/exclusiveLocks.feature#L57)
-   [coreApiWebdavLocks/exclusiveLocks.feature:58](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks/exclusiveLocks.feature#L58)
-   [coreApiWebdavLocks/exclusiveLocks.feature:76](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks/exclusiveLocks.feature#L76)
-   [coreApiWebdavLocks/exclusiveLocks.feature:77](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks/exclusiveLocks.feature#L77)
-   [coreApiWebdavLocks/exclusiveLocks.feature:78](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks/exclusiveLocks.feature#L78)
-   [coreApiWebdavLocks/exclusiveLocks.feature:79](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks/exclusiveLocks.feature#L79)
-   [coreApiWebdavLocks/exclusiveLocks.feature:84](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks/exclusiveLocks.feature#L84)
-   [coreApiWebdavLocks/exclusiveLocks.feature:85](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks/exclusiveLocks.feature#L85)
-   [coreApiWebdavLocks/exclusiveLocks.feature:103](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks/exclusiveLocks.feature#L103)
-   [coreApiWebdavLocks/exclusiveLocks.feature:104](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks/exclusiveLocks.feature#L104)
-   [coreApiWebdavLocks/exclusiveLocks.feature:105](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks/exclusiveLocks.feature#L105)
-   [coreApiWebdavLocks/exclusiveLocks.feature:106](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks/exclusiveLocks.feature#L106)
-   [coreApiWebdavLocks/exclusiveLocks.feature:111](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks/exclusiveLocks.feature#L111)
-   [coreApiWebdavLocks/exclusiveLocks.feature:112](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks/exclusiveLocks.feature#L112)
-   [coreApiWebdavLocks/requestsWithToken.feature:32](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks/requestsWithToken.feature#L32)
-   [coreApiWebdavLocks/requestsWithToken.feature:33](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks/requestsWithToken.feature#L33)
-   [coreApiWebdavLocks/requestsWithToken.feature:38](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks/requestsWithToken.feature#L38)
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
-   [coreApiWebdavLocks2/independentLocksShareToShares.feature:30](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks2/independentLocksShareToShares.feature#L30)
-   [coreApiWebdavLocks2/independentLocksShareToShares.feature:31](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks2/independentLocksShareToShares.feature#L31)
-   [coreApiWebdavLocks2/independentLocksShareToShares.feature:32](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks2/independentLocksShareToShares.feature#L32)
-   [coreApiWebdavLocks2/independentLocksShareToShares.feature:33](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks2/independentLocksShareToShares.feature#L33)
-   [coreApiWebdavLocks2/independentLocksShareToShares.feature:38](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks2/independentLocksShareToShares.feature#L38)
-   [coreApiWebdavLocks2/independentLocksShareToShares.feature:39](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks2/independentLocksShareToShares.feature#L39)
-   [coreApiWebdavLocks2/independentLocksShareToShares.feature:59](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks2/independentLocksShareToShares.feature#L59)
-   [coreApiWebdavLocks2/independentLocksShareToShares.feature:60](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks2/independentLocksShareToShares.feature#L60)
-   [coreApiWebdavLocks2/independentLocksShareToShares.feature:61](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks2/independentLocksShareToShares.feature#L61)
-   [coreApiWebdavLocks2/independentLocksShareToShares.feature:62](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks2/independentLocksShareToShares.feature#L62)
-   [coreApiWebdavLocks2/independentLocksShareToShares.feature:67](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks2/independentLocksShareToShares.feature#L67)
-   [coreApiWebdavLocks2/independentLocksShareToShares.feature:68](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks2/independentLocksShareToShares.feature#L68)
-   [coreApiWebdavLocksUnlock/unlock.feature:23](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlock.feature#L23)
-   [coreApiWebdavLocksUnlock/unlock.feature:24](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlock.feature#L24)
-   [coreApiWebdavLocksUnlock/unlock.feature:29](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlock.feature#L29)
-   [coreApiWebdavLocksUnlock/unlock.feature:43](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlock.feature#L43)
-   [coreApiWebdavLocksUnlock/unlock.feature:44](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlock.feature#L44)
-   [coreApiWebdavLocksUnlock/unlock.feature:66](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlock.feature#L66)
-   [coreApiWebdavLocksUnlock/unlock.feature:67](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlock.feature#L67)
-   [coreApiWebdavLocksUnlock/unlock.feature:68](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlock.feature#L68)
-   [coreApiWebdavLocksUnlock/unlock.feature:69](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlock.feature#L69)
-   [coreApiWebdavLocksUnlock/unlock.feature:74](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlock.feature#L74)
-   [coreApiWebdavLocksUnlock/unlock.feature:75](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlock.feature#L75)
-   [coreApiWebdavLocksUnlock/unlockSharingToShares.feature:28](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlockSharingToShares.feature#L28)
-   [coreApiWebdavLocksUnlock/unlockSharingToShares.feature:29](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlockSharingToShares.feature#L29)
-   [coreApiWebdavLocksUnlock/unlockSharingToShares.feature:30](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlockSharingToShares.feature#L30)
-   [coreApiWebdavLocksUnlock/unlockSharingToShares.feature:31](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlockSharingToShares.feature#L31)
-   [coreApiWebdavLocksUnlock/unlockSharingToShares.feature:36](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlockSharingToShares.feature#L36)
-   [coreApiWebdavLocksUnlock/unlockSharingToShares.feature:37](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlockSharingToShares.feature#L37)
-   [coreApiWebdavLocksUnlock/unlockSharingToShares.feature:52](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlockSharingToShares.feature#L52)
-   [coreApiWebdavLocksUnlock/unlockSharingToShares.feature:53](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlockSharingToShares.feature#L53)
-   [coreApiWebdavLocksUnlock/unlockSharingToShares.feature:54](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlockSharingToShares.feature#L54)
-   [coreApiWebdavLocksUnlock/unlockSharingToShares.feature:55](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlockSharingToShares.feature#L55)
-   [coreApiWebdavLocksUnlock/unlockSharingToShares.feature:60](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlockSharingToShares.feature#L60)
-   [coreApiWebdavLocksUnlock/unlockSharingToShares.feature:61](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlockSharingToShares.feature#L61)
-   [coreApiWebdavLocksUnlock/unlockSharingToShares.feature:76](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlockSharingToShares.feature#L76)
-   [coreApiWebdavLocksUnlock/unlockSharingToShares.feature:77](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlockSharingToShares.feature#L77)
-   [coreApiWebdavLocksUnlock/unlockSharingToShares.feature:78](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlockSharingToShares.feature#L78)
-   [coreApiWebdavLocksUnlock/unlockSharingToShares.feature:79](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlockSharingToShares.feature#L79)
-   [coreApiWebdavLocksUnlock/unlockSharingToShares.feature:84](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlockSharingToShares.feature#L84)
-   [coreApiWebdavLocksUnlock/unlockSharingToShares.feature:85](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlockSharingToShares.feature#L85)
-   [coreApiWebdavLocksUnlock/unlockSharingToShares.feature:100](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlockSharingToShares.feature#L100)
-   [coreApiWebdavLocksUnlock/unlockSharingToShares.feature:101](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlockSharingToShares.feature#L101)
-   [coreApiWebdavLocksUnlock/unlockSharingToShares.feature:102](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlockSharingToShares.feature#L102)
-   [coreApiWebdavLocksUnlock/unlockSharingToShares.feature:103](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlockSharingToShares.feature#L103)
-   [coreApiWebdavLocksUnlock/unlockSharingToShares.feature:108](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlockSharingToShares.feature#L108)
-   [coreApiWebdavLocksUnlock/unlockSharingToShares.feature:109](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlockSharingToShares.feature#L109)
-   [coreApiWebdavLocksUnlock/unlockSharingToShares.feature:124](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlockSharingToShares.feature#L124)
-   [coreApiWebdavLocksUnlock/unlockSharingToShares.feature:125](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlockSharingToShares.feature#L125)
-   [coreApiWebdavLocksUnlock/unlockSharingToShares.feature:126](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlockSharingToShares.feature#L126)
-   [coreApiWebdavLocksUnlock/unlockSharingToShares.feature:127](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlockSharingToShares.feature#L127)
-   [coreApiWebdavLocksUnlock/unlockSharingToShares.feature:132](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlockSharingToShares.feature#L132)
-   [coreApiWebdavLocksUnlock/unlockSharingToShares.feature:133](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlockSharingToShares.feature#L133)

### Share
File and sync features in a shared scenario

### Contain the remotes list in response to getting sharees while searching
-   [coreApiSharees/sharees.feature:180](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharees/sharees.feature#L180)
-   [coreApiSharees/sharees.feature:181](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharees/sharees.feature#L181)

#### [accepting matching name shared resources from different users/groups sets no serial identifiers on the resource name for the receiver](https://github.com/owncloud/ocis/issues/4289)
-   [coreApiShareManagementToShares/acceptShares.feature:228](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementToShares/acceptShares.feature#L228)
-   [coreApiShareManagementToShares/acceptShares.feature:238](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementToShares/acceptShares.feature#L238)

#### [Getting content of a shared file with same name returns 500](https://github.com/owncloud/ocis/issues/3880)
-   [coreApiShareManagementToShares/acceptShares.feature:459](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementToShares/acceptShares.feature#L459)
-   [coreApiShareManagementToShares/acceptShares.feature:524](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementToShares/acceptShares.feature#L524)
-   [coreApiShareCreateSpecialToShares2/createShareReceivedInMultipleWays.feature:130](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareCreateSpecialToShares2/createShareReceivedInMultipleWays.feature#L130)
-   [coreApiShareCreateSpecialToShares2/createShareReceivedInMultipleWays.feature:131](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareCreateSpecialToShares2/createShareReceivedInMultipleWays.feature#L131)
-   [coreApiShareCreateSpecialToShares2/createShareReceivedInMultipleWays.feature:165](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareCreateSpecialToShares2/createShareReceivedInMultipleWays.feature#L165)
-   [coreApiShareCreateSpecialToShares2/createShareReceivedInMultipleWays.feature:166](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareCreateSpecialToShares2/createShareReceivedInMultipleWays.feature#L166)
-   [coreApiShareCreateSpecialToShares2/createShareReceivedInMultipleWays.feature:39](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareCreateSpecialToShares2/createShareReceivedInMultipleWays.feature#L39)
-   [coreApiShareCreateSpecialToShares2/createShareReceivedInMultipleWays.feature:40](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareCreateSpecialToShares2/createShareReceivedInMultipleWays.feature#L40)


#### [File deletion using dav gives unique string in filename in the trashbin](https://github.com/owncloud/product/issues/178)
-   [coreApiShareManagementBasicToShares/deleteShareFromShares.feature:64](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/deleteShareFromShares.feature#L64)
-   [coreApiShareManagementBasicToShares/deleteShareFromShares.feature:78](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/deleteShareFromShares.feature#L78)

 cannot share a folder with create permission
#### [Listing shares via ocs API does not show path for parent folders](https://github.com/owncloud/ocis/issues/1231)
-   [coreApiShareManagementBasicToShares/deleteShareFromShares.feature:131](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/deleteShareFromShares.feature#L131)
-   [coreApiShareManagementBasicToShares/deleteShareFromShares.feature:144](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/deleteShareFromShares.feature#L144)
-   [coreApiShareManagementBasicToShares/deleteShareFromShares.feature:180](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/deleteShareFromShares.feature#L180)
-   [coreApiShareManagementBasicToShares/deleteShareFromShares.feature:181](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/deleteShareFromShares.feature#L181)
-   [coreApiShareManagementBasicToShares/deleteShareFromShares.feature:178](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/deleteShareFromShares.feature#L178)
-   [coreApiShareManagementBasicToShares/deleteShareFromShares.feature:179](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/deleteShareFromShares.feature#L179)
-   [coreApiShareManagementBasicToShares/deleteShareFromShares.feature:199](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/deleteShareFromShares.feature#L199)
-   [coreApiShareManagementBasicToShares/deleteShareFromShares.feature:200](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/deleteShareFromShares.feature#L200)
-   [coreApiShareManagementBasicToShares/deleteShareFromShares.feature:197](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/deleteShareFromShares.feature#L197)
-   [coreApiShareManagementBasicToShares/deleteShareFromShares.feature:198](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/deleteShareFromShares.feature#L198)
-   [coreApiShareOperationsToShares1/gettingShares.feature:189](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares1/gettingShares.feature#L189)
-   [coreApiShareOperationsToShares1/gettingShares.feature:190](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares1/gettingShares.feature#L190)

#### [OCS error message for attempting to access share via share id as an unauthorized user is not informative](https://github.com/owncloud/ocis/issues/1233)

-   [coreApiShareOperationsToShares1/gettingShares.feature:152](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares1/gettingShares.feature#L152)
-   [coreApiShareOperationsToShares1/gettingShares.feature:153](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares1/gettingShares.feature#L153)

#### [Public link enforce permissions](https://github.com/owncloud/ocis/issues/1269)

-   [coreApiSharePublicLink1/accessToPublicLinkShare.feature:13](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharePublicLink1/accessToPublicLinkShare.feature#L13)
-   [coreApiSharePublicLink1/accessToPublicLinkShare.feature:32](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharePublicLink1/accessToPublicLinkShare.feature#L32)
-   [coreApiSharePublicLink1/createPublicLinkShare.feature:327](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharePublicLink1/createPublicLinkShare.feature#L327)


#### [Sharing of project space root via public link does no longer work](https://github.com/owncloud/ocis/issues/6278)
-   [coreApiShareCreateSpecialToShares2/createShareDefaultFolderForReceivedShares.feature:23](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareCreateSpecialToShares2/createShareDefaultFolderForReceivedShares.feature#L23)
-   [coreApiShareCreateSpecialToShares2/createShareDefaultFolderForReceivedShares.feature:24](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareCreateSpecialToShares2/createShareDefaultFolderForReceivedShares.feature#L24)

#### [copying a folder within a public link folder to folder with same name as an already existing file overwrites the parent file](https://github.com/owncloud/ocis/issues/1232)

- [coreApiSharePublicLink2/copyFromPublicLink.feature:66](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharePublicLink2/copyFromPublicLink.feature#L66)
- [coreApiSharePublicLink2/copyFromPublicLink.feature:92](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharePublicLink2/copyFromPublicLink.feature#L92)
- [coreApiSharePublicLink2/copyFromPublicLink.feature:176](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharePublicLink2/copyFromPublicLink.feature#L176)
- [coreApiSharePublicLink2/copyFromPublicLink.feature:177](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharePublicLink2/copyFromPublicLink.feature#L177)
- [coreApiSharePublicLink2/copyFromPublicLink.feature:192](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharePublicLink2/copyFromPublicLink.feature#L192)
- [coreApiSharePublicLink2/copyFromPublicLink.feature:193](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharePublicLink2/copyFromPublicLink.feature#L193)

#### [Upload-only shares must not overwrite but create a separate file](https://github.com/owncloud/ocis/issues/1267)

- [coreApiSharePublicLink3/uploadToPublicLinkShare.feature:13](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharePublicLink3/uploadToPublicLinkShare.feature#L13)
- [coreApiSharePublicLink3/uploadToPublicLinkShare.feature:114](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharePublicLink3/uploadToPublicLinkShare.feature#L114)

#### [Set quota over settings](https://github.com/owncloud/ocis/issues/1290)
_requires a [CS3 user provisioning api that can update the quota for a user](https://github.com/cs3org/cs3apis/pull/95#issuecomment-772780683)_

- [coreApiSharePublicLink3/uploadToPublicLinkShare.feature:87](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharePublicLink3/uploadToPublicLinkShare.feature#L87)
- [coreApiSharePublicLink3/uploadToPublicLinkShare.feature:96](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharePublicLink3/uploadToPublicLinkShare.feature#L96)


#### [path property in pending shares gives only filename](https://github.com/owncloud/ocis/issues/2156)
-   [coreApiShareManagementBasicToShares/createShareToSharesFolder.feature:743](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/createShareToSharesFolder.feature#L743)
-   [coreApiShareManagementBasicToShares/createShareToSharesFolder.feature:744](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/createShareToSharesFolder.feature#L744)
-   [coreApiShareManagementBasicToShares/createShareToSharesFolder.feature:762](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/createShareToSharesFolder.feature#L762)
-   [coreApiShareManagementBasicToShares/createShareToSharesFolder.feature:763](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/createShareToSharesFolder.feature#L763)
-   [coreApiShareManagementBasicToShares/createShareToSharesFolder.feature:778](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/createShareToSharesFolder.feature#L778)
-   [coreApiShareManagementBasicToShares/createShareToSharesFolder.feature:779](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/createShareToSharesFolder.feature#L779)

#### [deleting a file inside a received shared folder is moved to the trash-bin of the sharer not the receiver](https://github.com/owncloud/ocis/issues/1124)
-   [coreApiTrashbin/trashbinSharingToShares.feature:30](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinSharingToShares.feature#L30)
-   [coreApiTrashbin/trashbinSharingToShares.feature:47](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinSharingToShares.feature#L47)
-   [coreApiTrashbin/trashbinSharingToShares.feature:52](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinSharingToShares.feature#L52)
-   [coreApiTrashbin/trashbinSharingToShares.feature:74](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinSharingToShares.feature#L74)
-   [coreApiTrashbin/trashbinSharingToShares.feature:79](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinSharingToShares.feature#L79)
-   [coreApiTrashbin/trashbinSharingToShares.feature:101](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinSharingToShares.feature#L101)
-   [coreApiTrashbin/trashbinSharingToShares.feature:106](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinSharingToShares.feature#L106)
-   [coreApiTrashbin/trashbinSharingToShares.feature:129](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinSharingToShares.feature#L129)
-   [coreApiTrashbin/trashbinSharingToShares.feature:134](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinSharingToShares.feature#L134)
-   [coreApiTrashbin/trashbinSharingToShares.feature:157](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinSharingToShares.feature#L157)
-   [coreApiTrashbin/trashbinSharingToShares.feature:162](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinSharingToShares.feature#L162)
-   [coreApiTrashbin/trashbinSharingToShares.feature:185](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinSharingToShares.feature#L185)
-   [coreApiTrashbin/trashbinSharingToShares.feature:190](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinSharingToShares.feature#L190)
-   [coreApiTrashbin/trashbinSharingToShares.feature:213](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinSharingToShares.feature#L213)
-   [coreApiTrashbin/trashbinSharingToShares.feature:237](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinSharingToShares.feature#L237)

#### [oc:privatelink property not returned in webdav responses](https://github.com/owncloud/product/issues/262)
-   [coreApiWebdavProperties2/getFileProperties.feature:294](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties2/getFileProperties.feature#L294)
-   [coreApiWebdavProperties2/getFileProperties.feature:295](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties2/getFileProperties.feature#L295)
-   [coreApiWebdavProperties2/getFileProperties.feature:300](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties2/getFileProperties.feature#L300)

#### [changing user quota gives ocs status 103 / Cannot set quota](https://github.com/owncloud/product/issues/247)
_requires a [CS3 user provisioning api that can update the quota for a user](https://github.com/cs3org/cs3apis/pull/95#issuecomment-772780683)_
-   [coreApiShareOperationsToShares2/uploadToShare.feature:211](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/uploadToShare.feature#L211)
-   [coreApiShareOperationsToShares2/uploadToShare.feature:212](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/uploadToShare.feature#L212)

#### [not possible to move file into a received folder](https://github.com/owncloud/ocis/issues/764)
-   [coreApiShareOperationsToShares1/changingFilesShare.feature:26](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares1/changingFilesShare.feature#L26)
-   [coreApiShareOperationsToShares1/changingFilesShare.feature:27](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares1/changingFilesShare.feature#L27)
-   [coreApiShareOperationsToShares1/changingFilesShare.feature:70](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares1/changingFilesShare.feature#L70)
-   [coreApiShareOperationsToShares1/changingFilesShare.feature:71](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares1/changingFilesShare.feature#L71)
-   [coreApiShareOperationsToShares1/changingFilesShare.feature:92](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares1/changingFilesShare.feature#L92)
-   [coreApiShareOperationsToShares1/changingFilesShare.feature:93](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares1/changingFilesShare.feature#L93)
-   [coreApiShareManagementBasicToShares/createShareToSharesFolder.feature:534](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/createShareToSharesFolder.feature#L534)
-   [coreApiWebdavMove2/moveShareOnOcis.feature:30](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove2/moveShareOnOcis.feature#L30)
-   [coreApiWebdavMove2/moveShareOnOcis.feature:32](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove2/moveShareOnOcis.feature#L32)
-   [coreApiWebdavMove2/moveShareOnOcis.feature:98](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove2/moveShareOnOcis.feature#L98)
-   [coreApiWebdavMove2/moveShareOnOcis.feature:100](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove2/moveShareOnOcis.feature#L100)
-   [coreApiWebdavMove2/moveShareOnOcis.feature:169](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove2/moveShareOnOcis.feature#L169)
-   [coreApiWebdavMove2/moveShareOnOcis.feature:170](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove2/moveShareOnOcis.feature#L170)

#### [Expiration date for shares is not implemented](https://github.com/owncloud/ocis/issues/1250)
#### Expiration date of user shares
- [coreApiShareReshareToShares3/reShareWithExpiryDate.feature:35](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares3/reShareWithExpiryDate.feature#L35)
- [coreApiShareReshareToShares3/reShareWithExpiryDate.feature:36](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares3/reShareWithExpiryDate.feature#L36)

#### Expiration date of group shares
-   [coreApiShareReshareToShares3/reShareWithExpiryDate.feature:60](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares3/reShareWithExpiryDate.feature#L60)
-   [coreApiShareReshareToShares3/reShareWithExpiryDate.feature:61](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares3/reShareWithExpiryDate.feature#L61)

#### [Empty OCS response for a share create request using a disabled user](https://github.com/owncloud/ocis/issues/2212)
-   [coreApiShareCreateSpecialToShares2/createShareWithDisabledUser.feature:21](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareCreateSpecialToShares2/createShareWithDisabledUser.feature#L21)
-   [coreApiShareCreateSpecialToShares2/createShareWithDisabledUser.feature:24](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareCreateSpecialToShares2/createShareWithDisabledUser.feature#L24)

#### [Edit user share response has an "name" field](https://github.com/owncloud/ocis/issues/1225)
- [coreApiShareUpdateToShares/updateShare.feature:243](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareUpdateToShares/updateShare.feature#L243)
- [coreApiShareUpdateToShares/updateShare.feature:244](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareUpdateToShares/updateShare.feature#L244)

#### [Share lists deleted user as 'user'](https://github.com/owncloud/ocis/issues/903)
-   [coreApiShareManagementBasicToShares/createShareToSharesFolder.feature:678](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/createShareToSharesFolder.feature#L678)
-   [coreApiShareManagementBasicToShares/createShareToSharesFolder.feature:679](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/createShareToSharesFolder.feature#L679)

#### [deleting a share with wrong authentication returns OCS status 996 / HTTP 500](https://github.com/owncloud/ocis/issues/1229)
-   [coreApiShareManagementBasicToShares/deleteShareFromShares.feature:227](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/deleteShareFromShares.feature#L227)
-   [coreApiShareManagementBasicToShares/deleteShareFromShares.feature:228](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/deleteShareFromShares.feature#L228)

### User Management
User and group management features

### Other
API, search, favorites, config, capabilities, not existing endpoints, CORS and others

#### [no robots.txt available](https://github.com/owncloud/ocis/issues/1314)
-   [coreApiMain/main.feature:8](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiMain/main.feature#L8) Scenario: robots.txt file should be accessible

#### [Ability to return error messages in Webdav response bodies](https://github.com/owncloud/ocis/issues/1293)
-   [coreApiAuthOcs/ocsDELETEAuth.feature:11](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthOcs/ocsDELETEAuth.feature#L11) Scenario: send DELETE requests to OCS endpoints as admin with wrong password
-   [coreApiAuthOcs/ocsGETAuth.feature:11](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthOcs/ocsGETAuth.feature#L11) Scenario: using OCS anonymously
-   [coreApiAuthOcs/ocsGETAuth.feature:45](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthOcs/ocsGETAuth.feature#L45) Scenario: using OCS with non-admin basic auth
-   [coreApiAuthOcs/ocsGETAuth.feature:76](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthOcs/ocsGETAuth.feature#L76) Scenario: using OCS as normal user with wrong password
-   [coreApiAuthOcs/ocsGETAuth.feature:107](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthOcs/ocsGETAuth.feature#L107) Scenario:using OCS with admin basic auth
-   [coreApiAuthOcs/ocsGETAuth.feature:124](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthOcs/ocsGETAuth.feature#L124) Scenario: using OCS as admin user with wrong password
-   [coreApiAuthOcs/ocsPOSTAuth.feature:11](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthOcs/ocsPOSTAuth.feature#L11) Scenario: send POST requests to OCS endpoints as normal user with wrong password
-   [coreApiAuthOcs/ocsPUTAuth.feature:11](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthOcs/ocsPUTAuth.feature#L11) Scenario: send PUT request to OCS endpoints as admin with wrong password
-   [coreApiSharePublicLink1/createPublicLinkShare.feature:317](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharePublicLink1/createPublicLinkShare.feature#L317)

#### [sending MKCOL requests to another or non-existing user's webDav endpoints as normal user should return 404](https://github.com/owncloud/ocis/issues/5049)
_ocdav: api compatibility, return correct status code_
-   [coreApiAuthWebDav/webDavMKCOLAuth.feature:43](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthWebDav/webDavMKCOLAuth.feature#L43) Scenario: send MKCOL requests to another user's webDav endpoints as normal user
-   [coreApiAuthWebDav/webDavMKCOLAuth.feature:54](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthWebDav/webDavMKCOLAuth.feature#L54) Scenario: send MKCOL requests to another user's webDav endpoints as normal user using the spaces WebDAV API

#### [trying to lock file of another user gives http 200](https://github.com/owncloud/ocis/issues/2176)
-   [coreApiAuthWebDav/webDavLOCKAuth.feature:47](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthWebDav/webDavLOCKAuth.feature#L47) Scenario: send LOCK requests to another user's webDav endpoints as normal user
-   [coreApiAuthWebDav/webDavLOCKAuth.feature:59](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthWebDav/webDavLOCKAuth.feature#L59) Scenario: send LOCK requests to another user's webDav endpoints as normal user using the spaces WebDAV API

#### [send (MOVE, COPY) requests to another user's webDav endpoints as normal user gives 400 instead of 403](https://github.com/owncloud/ocis/issues/3882)
_ocdav: api compatibility, return correct status code_
-   [coreApiAuthWebDav/webDavMOVEAuth.feature:46](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthWebDav/webDavMOVEAuth.feature#L46) Scenario: send MOVE requests to another user's webDav endpoints as normal user
-   [coreApiAuthWebDav/webDavMOVEAuth.feature:55](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthWebDav/webDavMOVEAuth.feature#L55) Scenario: send MOVE requests to another user's webDav endpoints as normal user using the spaces WebDAV API
-   [coreApiAuthWebDav/webDavCOPYAuth.feature:46](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthWebDav/webDavCOPYAuth.feature#L46)
-   [coreApiAuthWebDav/webDavCOPYAuth.feature:55](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthWebDav/webDavCOPYAuth.feature#L55)

#### [send POST requests to another user's webDav endpoints as normal user](https://github.com/owncloud/ocis/issues/1287)
_ocdav: api compatibility, return correct status code_
-   [coreApiAuthWebDav/webDavPOSTAuth.feature:47](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthWebDav/webDavPOSTAuth.feature#L47)  Scenario: send POST requests to another user's webDav endpoints as normal user
-   [coreApiAuthWebDav/webDavPOSTAuth.feature:56](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthWebDav/webDavPOSTAuth.feature#L56)  Scenario: send POST requests to another user's webDav endpoints as normal user using the spaces WebDAV API

#### [Using double slash in URL to access a folder gives 501 and other status codes](https://github.com/owncloud/ocis/issues/1667)
-   [coreApiAuthWebDav/webDavSpecialURLs.feature:124](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthWebDav/webDavSpecialURLs.feature#L124)
-   [coreApiAuthWebDav/webDavSpecialURLs.feature:135](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthWebDav/webDavSpecialURLs.feature#L135)
-   [coreApiAuthWebDav/webDavSpecialURLs.feature:166](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthWebDav/webDavSpecialURLs.feature#L166)
-   [coreApiAuthWebDav/webDavSpecialURLs.feature:177](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthWebDav/webDavSpecialURLs.feature#L177)

#### [Difference in response content of status.php and default capabilities](https://github.com/owncloud/ocis/issues/1286)
-   [coreApiCapabilities/capabilitiesWithNormalUser.feature:14](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiCapabilities/capabilitiesWithNormalUser.feature#L14) Scenario: getting default capabilities with normal user

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
-   [coreApiWebdavOperations/search.feature:108](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L108)
-   [coreApiWebdavOperations/search.feature:126](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L126)
-   [coreApiWebdavOperations/search.feature:127](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L127)
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
-   [coreApiFavorites/favorites.feature:118](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favorites.feature#L118)
-   [coreApiFavorites/favorites.feature:119](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favorites.feature#L119)
-   [coreApiFavorites/favorites.feature:170](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favorites.feature#L170)
-   [coreApiFavorites/favorites.feature:171](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favorites.feature#L171)
-   [coreApiFavorites/favorites.feature:203](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favorites.feature#L203)
-   [coreApiFavorites/favorites.feature:204](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favorites.feature#L204)
-   [coreApiFavorites/favorites.feature:176](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favorites.feature#L176)
-   [coreApiFavorites/favorites.feature:209](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favorites.feature#L209)
-   [coreApiFavorites/favorites.feature:222](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favorites.feature#L222)
-   [coreApiFavorites/favorites.feature:223](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favorites.feature#L223)

#### [resource inside Shares dir is not found using the spaces WebDAV API](https://github.com/owncloud/ocis/issues/2968)
-   [coreApiFavorites/favoritesSharingToShares.feature:68](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favoritesSharingToShares.feature#L68)
-   [coreApiFavorites/favoritesSharingToShares.feature:69](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favoritesSharingToShares.feature#L69)


#### [WWW-Authenticate header for unauthenticated requests is not clear](https://github.com/owncloud/ocis/issues/2285)
-   [coreApiWebdavOperations/refuseAccess.feature:22](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/refuseAccess.feature#L22)
-   [coreApiWebdavOperations/refuseAccess.feature:23](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/refuseAccess.feature#L23)
-   [coreApiWebdavOperations/refuseAccess.feature:35](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/refuseAccess.feature#L35)
-   [coreApiWebdavOperations/refuseAccess.feature:36](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/refuseAccess.feature#L36)
-   [coreApiWebdavOperations/refuseAccess.feature:41](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/refuseAccess.feature#L41)

#### [Sharing a same file twice to the same group](https://github.com/owncloud/ocis/issues/1710)
-   [coreApiShareManagementBasicToShares/createShareToSharesFolder.feature:726](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/createShareToSharesFolder.feature#L726)
-   [coreApiShareManagementBasicToShares/createShareToSharesFolder.feature:727](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/createShareToSharesFolder.feature#L727)

#### [Request to edit non-existing user by authorized admin gets unauthorized in http response](https://github.com/owncloud/ocis/issues/38423)
-   [coreApiAuthOcs/ocsPUTAuth.feature:27](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthOcs/ocsPUTAuth.feature#L27)


### Won't fix
Not everything needs to be implemented for ocis. While the oc10 testsuite covers these things we are not looking at them right now.

* _The `OC-LazyOps` header is [no longer supported by the client](https://github.com/owncloud/client/pull/8398), implmenting this is not necessary for a first production release. We plan to have an uploed state machine to visualize the state of a file, see https://github.com/owncloud/ocis/issues/214_
* _Blacklisted ignored files are no longer required because ocis can handle `.htaccess` files without security implications introduced by serving user provided files with apache._

#### [Blacklist files extensions](https://github.com/owncloud/ocis/issues/2177)
-   [coreApiWebdavProperties1/copyFile.feature:118](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/copyFile.feature#L118)
-   [coreApiWebdavProperties1/copyFile.feature:119](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/copyFile.feature#L119)
-   [coreApiWebdavProperties1/copyFile.feature:124](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/copyFile.feature#L124)
-   [coreApiWebdavProperties1/createFileFolder.feature:98](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/createFileFolder.feature#L98)
-   [coreApiWebdavProperties1/createFileFolder.feature:99](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/createFileFolder.feature#L99)
-   [coreApiWebdavProperties1/createFileFolder.feature:104](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/createFileFolder.feature#L104)
-   [coreApiWebdavUpload1/uploadFile.feature:181](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUpload1/uploadFile.feature#L181)
-   [coreApiWebdavUpload1/uploadFile.feature:182](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUpload1/uploadFile.feature#L182)
-   [coreApiWebdavUpload1/uploadFile.feature:187](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUpload1/uploadFile.feature#L187)
-   [coreApiWebdavMove2/moveFile.feature:176](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove2/moveFile.feature#L176)
-   [coreApiWebdavMove2/moveFile.feature:177](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove2/moveFile.feature#L177)
-   [coreApiWebdavMove2/moveFile.feature:182](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove2/moveFile.feature#L182)

#### [cannot set blacklisted file names](https://github.com/owncloud/product/issues/260)
-   [coreApiWebdavMove1/moveFolderToBlacklistedName.feature:21](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove1/moveFolderToBlacklistedName.feature#L21)
-   [coreApiWebdavMove1/moveFolderToBlacklistedName.feature:22](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove1/moveFolderToBlacklistedName.feature#L22)
-   [coreApiWebdavMove1/moveFolderToBlacklistedName.feature:27](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove1/moveFolderToBlacklistedName.feature#L27)
-   [coreApiWebdavMove2/moveFileToBlacklistedName.feature:19](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove2/moveFileToBlacklistedName.feature#L19)
-   [coreApiWebdavMove2/moveFileToBlacklistedName.feature:20](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove2/moveFileToBlacklistedName.feature#L20)

#### [file_target of a auto-renamed file is not correct directly after sharing](https://github.com/owncloud/ocis/issues/32322)
-   [coreApiShareManagementToShares/mergeShare.feature:111](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementToShares/mergeShare.feature#L111)

### To triage
_The below features have been added after I last categorized them. AFAICT they are bugs. @jfd_

#### [PATCH request for TUS upload with wrong checksum gives incorrect response](https://github.com/owncloud/ocis/issues/1755)
-   [coreApiWebdavUploadTUS/checksums.feature:87](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L87)
-   [coreApiWebdavUploadTUS/checksums.feature:88](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L88)
-   [coreApiWebdavUploadTUS/checksums.feature:89](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L89)
-   [coreApiWebdavUploadTUS/checksums.feature:90](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L90)
-   [coreApiWebdavUploadTUS/checksums.feature:95](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L95)
-   [coreApiWebdavUploadTUS/checksums.feature:96](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L96)
-   [coreApiWebdavUploadTUS/checksums.feature:176](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L176)
-   [coreApiWebdavUploadTUS/checksums.feature:177](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L177)
-   [coreApiWebdavUploadTUS/checksums.feature:182](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L182)
-   [coreApiWebdavUploadTUS/checksums.feature:229](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L229)
-   [coreApiWebdavUploadTUS/checksums.feature:230](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L230)
-   [coreApiWebdavUploadTUS/checksums.feature:231](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L231)
-   [coreApiWebdavUploadTUS/checksums.feature:232](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L232)
-   [coreApiWebdavUploadTUS/checksums.feature:237](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L237)
-   [coreApiWebdavUploadTUS/checksums.feature:238](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L238)
-   [coreApiWebdavUploadTUS/checksums.feature:285](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L285)
-   [coreApiWebdavUploadTUS/checksums.feature:286](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L286)
-   [coreApiWebdavUploadTUS/checksums.feature:287](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L287)
-   [coreApiWebdavUploadTUS/checksums.feature:288](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L288)
-   [coreApiWebdavUploadTUS/checksums.feature:293](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L293)
-   [coreApiWebdavUploadTUS/checksums.feature:294](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L294)
-   [coreApiWebdavUploadTUS/optionsRequest.feature:11](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/optionsRequest.feature#L11)
-   [coreApiWebdavUploadTUS/optionsRequest.feature:26](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/optionsRequest.feature#L26)
-   [coreApiWebdavUploadTUS/uploadToShare.feature:176](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/uploadToShare.feature#L176)
-   [coreApiWebdavUploadTUS/uploadToShare.feature:177](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/uploadToShare.feature#L177)
-   [coreApiWebdavUploadTUS/uploadToShare.feature:195](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/uploadToShare.feature#L195)
-   [coreApiWebdavUploadTUS/uploadToShare.feature:196](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/uploadToShare.feature#L196)
-   [coreApiWebdavUploadTUS/uploadToShare.feature:214](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/uploadToShare.feature#L214)
-   [coreApiWebdavUploadTUS/uploadToShare.feature:215](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/uploadToShare.feature#L215)
-   [coreApiWebdavUploadTUS/uploadToShare.feature:253](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/uploadToShare.feature#L253)
-   [coreApiWebdavUploadTUS/uploadToShare.feature:254](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/uploadToShare.feature#L254)
-   [coreApiWebdavUploadTUS/uploadToShare.feature:295](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/uploadToShare.feature#L295)
-   [coreApiWebdavUploadTUS/uploadToShare.feature:296](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/uploadToShare.feature#L296)

#### [TUS OPTIONS requests do not reply with TUS headers when invalid password](https://github.com/owncloud/ocis/issues/1012)
-   [coreApiWebdavUploadTUS/optionsRequest.feature:41](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/optionsRequest.feature#L41)
-   [coreApiWebdavUploadTUS/optionsRequest.feature:56](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/optionsRequest.feature#L56)

#### [incorrect ocs(v2) status value when getting info of share that does not exist should be 404, gives 998](https://github.com/owncloud/product/issues/250)
_ocs: api compatibility, return correct status code_
-   [coreApiShareOperationsToShares2/shareAccessByID.feature:49](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L49)
-   [coreApiShareOperationsToShares2/shareAccessByID.feature:50](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L50)
-   [coreApiShareOperationsToShares2/shareAccessByID.feature:51](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L51)
-   [coreApiShareOperationsToShares2/shareAccessByID.feature:52](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L52)
-   [coreApiShareOperationsToShares2/shareAccessByID.feature:53](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L53)
-   [coreApiShareOperationsToShares2/shareAccessByID.feature:54](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L54)
-   [coreApiShareOperationsToShares2/shareAccessByID.feature:55](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L55)
-   [coreApiShareOperationsToShares2/shareAccessByID.feature:56](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L56)

#### [Trying to accept a share with invalid ID gives incorrect OCS and HTTP status](https://github.com/owncloud/ocis/issues/2111)
-   [coreApiShareOperationsToShares2/shareAccessByID.feature:85](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L85)
-   [coreApiShareOperationsToShares2/shareAccessByID.feature:86](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L86)
-   [coreApiShareOperationsToShares2/shareAccessByID.feature:87](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L87)
-   [coreApiShareOperationsToShares2/shareAccessByID.feature:88](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L88)
-   [coreApiShareOperationsToShares2/shareAccessByID.feature:89](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L89)
-   [coreApiShareOperationsToShares2/shareAccessByID.feature:90](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L90)
-   [coreApiShareOperationsToShares2/shareAccessByID.feature:91](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L91)
-   [coreApiShareOperationsToShares2/shareAccessByID.feature:92](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L92)
-   [coreApiShareOperationsToShares2/shareAccessByID.feature:103](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L103)
-   [coreApiShareOperationsToShares2/shareAccessByID.feature:104](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L104)
-   [coreApiShareOperationsToShares2/shareAccessByID.feature:134](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L134)
-   [coreApiShareOperationsToShares2/shareAccessByID.feature:135](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L135)
-   [coreApiShareOperationsToShares2/shareAccessByID.feature:136](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L136)
-   [coreApiShareOperationsToShares2/shareAccessByID.feature:137](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L137)
-   [coreApiShareOperationsToShares2/shareAccessByID.feature:138](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L138)
-   [coreApiShareOperationsToShares2/shareAccessByID.feature:139](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L139)
-   [coreApiShareOperationsToShares2/shareAccessByID.feature:140](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L140)
-   [coreApiShareOperationsToShares2/shareAccessByID.feature:141](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L141)
-   [coreApiShareOperationsToShares2/shareAccessByID.feature:152](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L152)
-   [coreApiShareOperationsToShares2/shareAccessByID.feature:153](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L153)

#### [Cannot move folder/file from one received share to another](https://github.com/owncloud/ocis/issues/2442)
- [coreApiShareUpdateToShares/updateShare.feature:197](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareUpdateToShares/updateShare.feature#L197)
- [coreApiShareUpdateToShares/updateShare.feature:161](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareUpdateToShares/updateShare.feature#L161)
- [coreApiShareManagementToShares/mergeShare.feature:131](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementToShares/mergeShare.feature#L131)

#### [Sharing folder and sub-folder with same user but different permission,the permission of sub-folder is not obeyed ](https://github.com/owncloud/ocis/issues/2440)
-   [coreApiShareCreateSpecialToShares2/createShareReceivedInMultipleWays.feature:262](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareCreateSpecialToShares2/createShareReceivedInMultipleWays.feature#L262)
-   [coreApiShareCreateSpecialToShares2/createShareReceivedInMultipleWays.feature:297](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareCreateSpecialToShares2/createShareReceivedInMultipleWays.feature#L297)
-   [coreApiShareCreateSpecialToShares2/createShareReceivedInMultipleWays.feature:406](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareCreateSpecialToShares2/createShareReceivedInMultipleWays.feature#L406)
-   [coreApiShareCreateSpecialToShares2/createShareReceivedInMultipleWays.feature:441](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareCreateSpecialToShares2/createShareReceivedInMultipleWays.feature#L441)

#### [copying the file inside Shares folder returns 404](https://github.com/owncloud/ocis/issues/3874)
-   [coreApiWebdavProperties1/copyFile.feature:408](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/copyFile.feature#L408)
-   [coreApiWebdavProperties1/copyFile.feature:409](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/copyFile.feature#L409)
-   [coreApiWebdavProperties1/copyFile.feature:414](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/copyFile.feature#L414)
-   [coreApiWebdavProperties1/copyFile.feature:434](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/copyFile.feature#L434)
-   [coreApiWebdavProperties1/copyFile.feature:435](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/copyFile.feature#L435)
-   [coreApiWebdavProperties1/copyFile.feature:440](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/copyFile.feature#L440)

#### [file versions do not report the version author](https://github.com/owncloud/ocis/issues/2914)
-   [coreApiVersions/fileVersionAuthor.feature:15](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiVersions/fileVersionAuthor.feature#L15)
-   [coreApiVersions/fileVersionAuthor.feature:46](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiVersions/fileVersionAuthor.feature#L46)
-   [coreApiVersions/fileVersionAuthor.feature:73](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiVersions/fileVersionAuthor.feature#L73)
-   [coreApiVersions/fileVersionAuthor.feature:99](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiVersions/fileVersionAuthor.feature#L99)
-   [coreApiVersions/fileVersionAuthor.feature:132](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiVersions/fileVersionAuthor.feature#L132)
-   [coreApiVersions/fileVersionAuthor.feature:159](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiVersions/fileVersionAuthor.feature#L159)
-   [coreApiVersions/fileVersionAuthor.feature:190](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiVersions/fileVersionAuthor.feature#L190)
-   [coreApiVersions/fileVersionAuthor.feature:225](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiVersions/fileVersionAuthor.feature#L225)
-   [coreApiVersions/fileVersionAuthor.feature:277](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiVersions/fileVersionAuthor.feature#L277)
-   [coreApiVersions/fileVersionAuthor.feature:326](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiVersions/fileVersionAuthor.feature#L326)
-   [coreApiVersions/fileVersionAuthor.feature:347](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiVersions/fileVersionAuthor.feature#L347)

#### [Shares to deleted group listed in the response](https://github.com/owncloud/ocis/issues/2441)
-   [coreApiShareManagementBasicToShares/createShareToSharesFolder.feature:530](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/createShareToSharesFolder.feature#L530)
-   [coreApiShareManagementBasicToShares/createShareToSharesFolder.feature:531](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/createShareToSharesFolder.feature#L531)

### [Share path in the response is different between share states](https://github.com/owncloud/ocis/issues/2540)
-   [coreApiShareManagementToShares/acceptShares.feature:28](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementToShares/acceptShares.feature#L28)
-   [coreApiShareManagementToShares/acceptShares.feature:62](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementToShares/acceptShares.feature#L62)
-   [coreApiShareManagementToShares/acceptShares.feature:134](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementToShares/acceptShares.feature#L134)
-   [coreApiShareManagementToShares/acceptShares.feature:155](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementToShares/acceptShares.feature#L155)
-   [coreApiShareManagementToShares/acceptShares.feature:183](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementToShares/acceptShares.feature#L183)
-   [coreApiShareManagementToShares/acceptShares.feature:260](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementToShares/acceptShares.feature#L260)
-   [coreApiShareManagementToShares/acceptShares.feature:438](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementToShares/acceptShares.feature#L438)
-   [coreApiShareOperationsToShares2/shareAccessByID.feature:122](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L122)
-   [coreApiShareOperationsToShares2/shareAccessByID.feature:123](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L123)
-   [coreApiShareManagementToShares/acceptShares.feature:208](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementToShares/acceptShares.feature#L208)

### [Content-type is not multipart/byteranges when downloading file with Range Header](https://github.com/owncloud/ocis/issues/2677)
-   [coreApiWebdavOperations/downloadFile.feature:184](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/downloadFile.feature#L184)
-   [coreApiWebdavOperations/downloadFile.feature:185](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/downloadFile.feature#L185)
-   [coreApiWebdavOperations/downloadFile.feature:190](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/downloadFile.feature#L190)

### [send PUT requests to another user's webDav endpoints as normal user](https://github.com/owncloud/ocis/issues/2893)
-   [coreApiAuthWebDav/webDavPUTAuth.feature:47](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthWebDav/webDavPUTAuth.feature#L47)
-   [coreApiAuthWebDav/webDavPUTAuth.feature:59](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthWebDav/webDavPUTAuth.feature#L59)

#### [moveShareInsideAnotherShare behaves differently on oCIS than oC10](https://github.com/owncloud/ocis/issues/3047)
-   [coreApiShareManagementToShares/moveShareInsideAnotherShare.feature:23](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementToShares/moveShareInsideAnotherShare.feature#L23)
-   [coreApiShareManagementToShares/moveShareInsideAnotherShare.feature:43](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementToShares/moveShareInsideAnotherShare.feature#L43)
-   [coreApiShareManagementToShares/moveShareInsideAnotherShare.feature:57](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementToShares/moveShareInsideAnotherShare.feature#L57)


#### [Renaming resource to banned name is allowed in spaces webdav](https://github.com/owncloud/ocis/issues/3099)
-   [coreApiWebdavMove1/moveFolder.feature:27](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove1/moveFolder.feature#L27)
-   [coreApiWebdavMove1/moveFolder.feature:45](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove1/moveFolder.feature#L45)
-   [coreApiWebdavMove1/moveFolder.feature:63](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove1/moveFolder.feature#L63)
-   [coreApiWebdavMove2/moveFile.feature:138](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove2/moveFile.feature#L138)
-   [coreApiWebdavMove2/moveFileToBlacklistedName.feature:25](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove2/moveFileToBlacklistedName.feature#L25)

#### [REPORT method on spaces returns an incorrect d:href response](https://github.com/owncloud/ocis/issues/3111)
-   [coreApiFavorites/favorites.feature:124](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favorites.feature#L124)
-   [coreApiFavorites/favorites.feature:228](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favorites.feature#L228)

#### [Cannot disable the dav propfind depth infinity for resources](https://github.com/owncloud/ocis/issues/3720)
-   [coreApiWebdavOperations/listFiles.feature:356](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/listFiles.feature#L356)
-   [coreApiWebdavOperations/listFiles.feature:355](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/listFiles.feature#L355)
-   [coreApiWebdavOperations/listFiles.feature:361](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/listFiles.feature#L361)
-   [coreApiWebdavOperations/listFiles.feature:380](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/listFiles.feature#L380)
-   [coreApiWebdavOperations/listFiles.feature:375](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/listFiles.feature#L375)
-   [coreApiWebdavOperations/listFiles.feature:394](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/listFiles.feature#L394)
-   [coreApiWebdavOperations/listFiles.feature:395](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/listFiles.feature#L395)
-   [coreApiWebdavOperations/listFiles.feature:400](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/listFiles.feature#L400)

#### [OCS status code zero](https://github.com/owncloud/ocis/issues/3621)
-   [coreApiShareManagementToShares/moveReceivedShare.feature:13](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementToShares/moveReceivedShare.feature#L13)

#### [coreApiShareOperationsToShares1/gettingShares.feature:28 fails in CI](https://github.com/cs3org/reva/issues/2926)
-   [coreApiShareOperationsToShares1/gettingShares.feature:40](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares1/gettingShares.feature#L40)
-   [coreApiShareOperationsToShares1/gettingShares.feature:41](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares1/gettingShares.feature#L41)

#### [HTTP status code differ while deleting file of another user's trash bin](https://github.com/owncloud/ocis/issues/3544)

-   [coreApiTrashbin/trashbinDelete.feature:105](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinDelete.feature#L105)

#### [Default capabilities for normal user and admin user not same as in oC-core](https://github.com/owncloud/ocis/issues/1285)
-   [coreApiCapabilities/capabilities.feature:11](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiCapabilities/capabilities.feature#L11)
-   [coreApiCapabilities/capabilities.feature:136](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiCapabilities/capabilities.feature#L136)
-   [coreApiCapabilities/capabilities.feature:175](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiCapabilities/capabilities.feature#L175)

- Note: always have an empty line at the end of this file.
The bash script that processes this file may not process a scenario reference on the last line.
