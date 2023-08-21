## Scenarios from OCIS API tests that are expected to fail with OCIS storage

The expected failures in this file are from features in the owncloud/ocis repo.

### File
Basic file management like up and download, move, copy, properties, quota, trash, versions and chunking.

#### [invalid webdav responses for unauthorized requests.](https://github.com/owncloud/product/issues/273)
- [coreApiTrashbin/trashbinFilesFolders.feature:235](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinFilesFolders.feature#L235)
- [coreApiTrashbin/trashbinFilesFolders.feature:268](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinFilesFolders.feature#L268)

### [Downloading the older version of shared file gives 404](https://github.com/owncloud/ocis/issues/3868)
- [coreApiVersions/fileVersions.feature:158](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiVersions/fileVersions.feature#L158)
- [coreApiVersions/fileVersions.feature:176](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiVersions/fileVersions.feature#L176)
- [coreApiVersions/fileVersions.feature:443](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiVersions/fileVersions.feature#L443)

#### [file versions do not report the version author](https://github.com/owncloud/ocis/issues/2914)
- [coreApiVersions/fileVersionAuthor.feature:15](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiVersions/fileVersionAuthor.feature#L15)
- [coreApiVersions/fileVersionAuthor.feature:46](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiVersions/fileVersionAuthor.feature#L46)
- [coreApiVersions/fileVersionAuthor.feature:73](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiVersions/fileVersionAuthor.feature#L73)
- [coreApiVersions/fileVersionAuthor.feature:99](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiVersions/fileVersionAuthor.feature#L99)
- [coreApiVersions/fileVersionAuthor.feature:132](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiVersions/fileVersionAuthor.feature#L132)
- [coreApiVersions/fileVersionAuthor.feature:159](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiVersions/fileVersionAuthor.feature#L159)
- [coreApiVersions/fileVersionAuthor.feature:190](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiVersions/fileVersionAuthor.feature#L190)
- [coreApiVersions/fileVersionAuthor.feature:225](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiVersions/fileVersionAuthor.feature#L225)
- [coreApiVersions/fileVersionAuthor.feature:277](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiVersions/fileVersionAuthor.feature#L277)
- [coreApiVersions/fileVersionAuthor.feature:326](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiVersions/fileVersionAuthor.feature#L326)
- [coreApiVersions/fileVersionAuthor.feature:347](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiVersions/fileVersionAuthor.feature#L347)

#### [Getting information about a folder overwritten by a file gives 500 error instead of 404](https://github.com/owncloud/ocis/issues/1239)
- [coreApiWebdavProperties1/copyFile.feature:272](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/copyFile.feature#L272)
- [coreApiWebdavProperties1/copyFile.feature:271](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/copyFile.feature#L271)
- [coreApiWebdavProperties1/copyFile.feature:290](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/copyFile.feature#L290)
- [coreApiWebdavProperties1/copyFile.feature:289](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/copyFile.feature#L289)
- [coreApiWebdavProperties1/copyFile.feature:313](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/copyFile.feature#L313)
- [coreApiWebdavProperties1/copyFile.feature:312](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/copyFile.feature#L312)
- [coreApiWebdavProperties1/copyFile.feature:338](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/copyFile.feature#L338)
- [coreApiWebdavProperties1/copyFile.feature:337](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/copyFile.feature#L337)
- [coreApiWebdavProperties1/copyFile.feature:362](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/copyFile.feature#L362)
- [coreApiWebdavProperties1/copyFile.feature:361](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/copyFile.feature#L361)
- [coreApiWebdavProperties1/copyFile.feature:386](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/copyFile.feature#L386)
- [coreApiWebdavProperties1/copyFile.feature:385](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/copyFile.feature#L385)
- [coreApiWebdavProperties1/copyFile.feature:465](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/copyFile.feature#L465)
- [coreApiWebdavProperties1/copyFile.feature:493](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/copyFile.feature#L493)
- [coreApiWebdavProperties1/copyFile.feature:492](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/copyFile.feature#L492)
- [coreApiWebdavProperties1/copyFile.feature:520](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/copyFile.feature#L520)
- [coreApiWebdavProperties1/copyFile.feature:519](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/copyFile.feature#L519)
- [coreApiWebdavProperties1/copyFile.feature:547](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/copyFile.feature#L547)

#### [Custom dav properties with namespaces are rendered incorrectly](https://github.com/owncloud/ocis/issues/2140)
_ocdav: double check the webdav property parsing when custom namespaces are used_
- [coreApiWebdavProperties1/setFileProperties.feature:36](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/setFileProperties.feature#L36)
- [coreApiWebdavProperties1/setFileProperties.feature:37](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/setFileProperties.feature#L37)
- [coreApiWebdavProperties1/setFileProperties.feature:77](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/setFileProperties.feature#L77)
- [coreApiWebdavProperties1/setFileProperties.feature:78](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/setFileProperties.feature#L78)

#### [Cannot set custom webDav properties](https://github.com/owncloud/product/issues/264)
- [coreApiWebdavProperties2/getFileProperties.feature:340](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties2/getFileProperties.feature#L340)
- [coreApiWebdavProperties2/getFileProperties.feature:341](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties2/getFileProperties.feature#L341)
- [coreApiWebdavProperties2/getFileProperties.feature:376](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties2/getFileProperties.feature#L376)
- [coreApiWebdavProperties2/getFileProperties.feature:377](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties2/getFileProperties.feature#L377)

### Sync
Synchronization features like etag propagation, setting mtime and locking files

#### [Uploading an old method chunked file with checksum should fail using new DAV path](https://github.com/owncloud/ocis/issues/2323)
- [coreApiMain/checksums.feature:260](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiMain/checksums.feature#L260)

#### [Webdav LOCK operations](https://github.com/owncloud/ocis/issues/1284)
- [coreApiWebdavLocks/exclusiveLocks.feature:22](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks/exclusiveLocks.feature#L22)
- [coreApiWebdavLocks/exclusiveLocks.feature:23](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks/exclusiveLocks.feature#L23)
- [coreApiWebdavLocks/exclusiveLocks.feature:24](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks/exclusiveLocks.feature#L24)
- [coreApiWebdavLocks/exclusiveLocks.feature:25](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks/exclusiveLocks.feature#L25)
- [coreApiWebdavLocks/exclusiveLocks.feature:49](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks/exclusiveLocks.feature#L49)
- [coreApiWebdavLocks/exclusiveLocks.feature:50](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks/exclusiveLocks.feature#L50)
- [coreApiWebdavLocks/exclusiveLocks.feature:51](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks/exclusiveLocks.feature#L51)
- [coreApiWebdavLocks/exclusiveLocks.feature:52](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks/exclusiveLocks.feature#L52)
- [coreApiWebdavLocks/exclusiveLocks.feature:76](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks/exclusiveLocks.feature#L76)
- [coreApiWebdavLocks/exclusiveLocks.feature:77](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks/exclusiveLocks.feature#L77)
- [coreApiWebdavLocks/exclusiveLocks.feature:78](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks/exclusiveLocks.feature#L78)
- [coreApiWebdavLocks/exclusiveLocks.feature:79](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks/exclusiveLocks.feature#L79)
- [coreApiWebdavLocks/exclusiveLocks.feature:103](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks/exclusiveLocks.feature#L103)
- [coreApiWebdavLocks/exclusiveLocks.feature:104](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks/exclusiveLocks.feature#L104)
- [coreApiWebdavLocks/exclusiveLocks.feature:105](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks/exclusiveLocks.feature#L105)
- [coreApiWebdavLocks/exclusiveLocks.feature:106](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks/exclusiveLocks.feature#L106)
- [coreApiWebdavLocks/requestsWithToken.feature:32](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks/requestsWithToken.feature#L32)
- [coreApiWebdavLocks/requestsWithToken.feature:33](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks/requestsWithToken.feature#L33)
- [coreApiWebdavLocks2/independentLocks.feature:25](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks2/independentLocks.feature#L25)
- [coreApiWebdavLocks2/independentLocks.feature:26](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks2/independentLocks.feature#L26)
- [coreApiWebdavLocks2/independentLocks.feature:27](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks2/independentLocks.feature#L27)
- [coreApiWebdavLocks2/independentLocks.feature:28](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks2/independentLocks.feature#L28)
- [coreApiWebdavLocks2/independentLocks.feature:53](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks2/independentLocks.feature#L53)
- [coreApiWebdavLocks2/independentLocks.feature:54](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks2/independentLocks.feature#L54)
- [coreApiWebdavLocks2/independentLocks.feature:55](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks2/independentLocks.feature#L55)
- [coreApiWebdavLocks2/independentLocks.feature:56](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks2/independentLocks.feature#L56)
- [coreApiWebdavLocks2/independentLocks.feature:57](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks2/independentLocks.feature#L57)
- [coreApiWebdavLocks2/independentLocks.feature:58](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks2/independentLocks.feature#L58)
- [coreApiWebdavLocks2/independentLocks.feature:59](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks2/independentLocks.feature#L59)
- [coreApiWebdavLocks2/independentLocks.feature:60](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks2/independentLocks.feature#L60)
- [coreApiWebdavLocks2/independentLocksShareToShares.feature:30](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks2/independentLocksShareToShares.feature#L30)
- [coreApiWebdavLocks2/independentLocksShareToShares.feature:31](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks2/independentLocksShareToShares.feature#L31)
- [coreApiWebdavLocks2/independentLocksShareToShares.feature:32](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks2/independentLocksShareToShares.feature#L32)
- [coreApiWebdavLocks2/independentLocksShareToShares.feature:33](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks2/independentLocksShareToShares.feature#L33)
- [coreApiWebdavLocks2/independentLocksShareToShares.feature:59](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks2/independentLocksShareToShares.feature#L59)
- [coreApiWebdavLocks2/independentLocksShareToShares.feature:60](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks2/independentLocksShareToShares.feature#L60)
- [coreApiWebdavLocks2/independentLocksShareToShares.feature:61](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks2/independentLocksShareToShares.feature#L61)
- [coreApiWebdavLocks2/independentLocksShareToShares.feature:62](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks2/independentLocksShareToShares.feature#L62)
- [coreApiWebdavLocksUnlock/unlock.feature:23](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlock.feature#L23)
- [coreApiWebdavLocksUnlock/unlock.feature:24](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlock.feature#L24)
- [coreApiWebdavLocksUnlock/unlock.feature:43](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlock.feature#L43)
- [coreApiWebdavLocksUnlock/unlock.feature:44](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlock.feature#L44)
- [coreApiWebdavLocksUnlock/unlock.feature:66](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlock.feature#L66)
- [coreApiWebdavLocksUnlock/unlock.feature:67](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlock.feature#L67)
- [coreApiWebdavLocksUnlock/unlock.feature:68](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlock.feature#L68)
- [coreApiWebdavLocksUnlock/unlock.feature:69](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlock.feature#L69)
- [coreApiWebdavLocksUnlock/unlockSharingToShares.feature:28](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlockSharingToShares.feature#L28)
- [coreApiWebdavLocksUnlock/unlockSharingToShares.feature:29](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlockSharingToShares.feature#L29)
- [coreApiWebdavLocksUnlock/unlockSharingToShares.feature:30](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlockSharingToShares.feature#L30)
- [coreApiWebdavLocksUnlock/unlockSharingToShares.feature:31](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlockSharingToShares.feature#L31)
- [coreApiWebdavLocksUnlock/unlockSharingToShares.feature:52](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlockSharingToShares.feature#L52)
- [coreApiWebdavLocksUnlock/unlockSharingToShares.feature:53](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlockSharingToShares.feature#L53)
- [coreApiWebdavLocksUnlock/unlockSharingToShares.feature:54](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlockSharingToShares.feature#L54)
- [coreApiWebdavLocksUnlock/unlockSharingToShares.feature:55](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlockSharingToShares.feature#L55)
- [coreApiWebdavLocksUnlock/unlockSharingToShares.feature:76](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlockSharingToShares.feature#L76)
- [coreApiWebdavLocksUnlock/unlockSharingToShares.feature:77](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlockSharingToShares.feature#L77)
- [coreApiWebdavLocksUnlock/unlockSharingToShares.feature:78](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlockSharingToShares.feature#L78)
- [coreApiWebdavLocksUnlock/unlockSharingToShares.feature:79](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlockSharingToShares.feature#L79)
- [coreApiWebdavLocksUnlock/unlockSharingToShares.feature:100](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlockSharingToShares.feature#L100)
- [coreApiWebdavLocksUnlock/unlockSharingToShares.feature:101](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlockSharingToShares.feature#L101)
- [coreApiWebdavLocksUnlock/unlockSharingToShares.feature:102](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlockSharingToShares.feature#L102)
- [coreApiWebdavLocksUnlock/unlockSharingToShares.feature:103](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlockSharingToShares.feature#L103)
- [coreApiWebdavLocksUnlock/unlockSharingToShares.feature:124](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlockSharingToShares.feature#L124)
- [coreApiWebdavLocksUnlock/unlockSharingToShares.feature:125](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlockSharingToShares.feature#L125)
- [coreApiWebdavLocksUnlock/unlockSharingToShares.feature:126](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlockSharingToShares.feature#L126)
- [coreApiWebdavLocksUnlock/unlockSharingToShares.feature:127](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlockSharingToShares.feature#L127)

### Share
File and sync features in a shared scenario

### [Different response containing exact and non exact match in response of getting sharees](https://github.com/owncloud/ocis/issues/2376)
- [coreApiSharees/sharees.feature:99](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharees/sharees.feature#L99)
- [coreApiSharees/sharees.feature:100](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharees/sharees.feature#L100)
- [coreApiSharees/sharees.feature:119](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharees/sharees.feature#L119)
- [coreApiSharees/sharees.feature:120](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharees/sharees.feature#L120)
- [coreApiSharees/sharees.feature:139](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharees/sharees.feature#L139)
- [coreApiSharees/sharees.feature:140](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharees/sharees.feature#L140)
- [coreApiSharees/sharees.feature:159](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharees/sharees.feature#L159)
- [coreApiSharees/sharees.feature:160](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharees/sharees.feature#L160)

#### [accepting matching name shared resources from different users/groups sets no serial identifiers on the resource name for the receiver](https://github.com/owncloud/ocis/issues/4289)
- [coreApiShareManagementToShares/acceptShares.feature:238](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementToShares/acceptShares.feature#L238)
- [coreApiShareManagementToShares/acceptShares.feature:260](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementToShares/acceptShares.feature#L260)

#### [Getting content of a shared file with same name returns 500](https://github.com/owncloud/ocis/issues/3880)
- [coreApiShareManagementToShares/acceptShares.feature:459](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementToShares/acceptShares.feature#L459)
- [coreApiShareManagementToShares/acceptShares.feature:524](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementToShares/acceptShares.feature#L524)
- [coreApiShareCreateSpecialToShares2/createShareReceivedInMultipleWays.feature:127](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareCreateSpecialToShares2/createShareReceivedInMultipleWays.feature#L127)
- [coreApiShareCreateSpecialToShares2/createShareReceivedInMultipleWays.feature:128](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareCreateSpecialToShares2/createShareReceivedInMultipleWays.feature#L128)
- [coreApiShareCreateSpecialToShares2/createShareReceivedInMultipleWays.feature:160](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareCreateSpecialToShares2/createShareReceivedInMultipleWays.feature#L160)
- [coreApiShareCreateSpecialToShares2/createShareReceivedInMultipleWays.feature:161](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareCreateSpecialToShares2/createShareReceivedInMultipleWays.feature#L161)
- [coreApiShareCreateSpecialToShares2/createShareReceivedInMultipleWays.feature:39](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareCreateSpecialToShares2/createShareReceivedInMultipleWays.feature#L39)
- [coreApiShareCreateSpecialToShares2/createShareReceivedInMultipleWays.feature:38](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareCreateSpecialToShares2/createShareReceivedInMultipleWays.feature#L38)

#### [file_target in share response](https://github.com/owncloud/product/issues/203) //todo

- [coreApiShareManagementBasicToShares/createShareToSharesFolder.feature:36](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/createShareToSharesFolder.feature#L36)
- [coreApiShareManagementBasicToShares/createShareToSharesFolder.feature:37](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/createShareToSharesFolder.feature#L37)
- [coreApiShareManagementBasicToShares/deleteShareFromShares.feature:61](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/deleteShareFromShares.feature#L61)

#### [different webdav permissions in ocis](https://github.com/owncloud/ocis/issues/4929)
- [coreApiShareManagementToShares/mergeShare.feature:98](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementToShares/mergeShare.feature#L98)

#### [file_target of a auto-renamed file is not correct directly after sharing](https://github.com/owncloud/ocis/issues/32322)

- [coreApiShareManagementToShares/mergeShare.feature:111](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementToShares/mergeShare.feature#L111)

#### [not possible to move file into a received folder](https://github.com/owncloud/ocis/issues/764)
- [coreApiShareManagementBasicToShares/createShareToSharesFolder.feature:528](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/createShareToSharesFolder.feature#L528)

#### [path property in pending shares gives only filename](https://github.com/owncloud/ocis/issues/2156)
- [coreApiShareManagementBasicToShares/createShareToSharesFolder.feature:715](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/createShareToSharesFolder.feature#L715)
- [coreApiShareManagementBasicToShares/createShareToSharesFolder.feature:716](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/createShareToSharesFolder.feature#L716)
- [coreApiShareManagementBasicToShares/createShareToSharesFolder.feature:732](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/createShareToSharesFolder.feature#L732)
- [coreApiShareManagementBasicToShares/createShareToSharesFolder.feature:733](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/createShareToSharesFolder.feature#L733)
- [coreApiShareManagementBasicToShares/createShareToSharesFolder.feature:751](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/createShareToSharesFolder.feature#L751)
- [coreApiShareManagementBasicToShares/createShareToSharesFolder.feature:752](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/createShareToSharesFolder.feature#L752)

#### [File deletion using dav gives unique string in filename in the trashbin](https://github.com/owncloud/product/issues/178)
- [coreApiShareManagementBasicToShares/deleteShareFromShares.feature:49](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/deleteShareFromShares.feature#L49)
- [coreApiShareManagementBasicToShares/deleteShareFromShares.feature:75](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/deleteShareFromShares.feature#L75)
 cannot share a folder with create permission
#### [Listing shares via ocs API does not show path for parent folders](https://github.com/owncloud/ocis/issues/1231)
- [coreApiShareManagementBasicToShares/deleteShareFromShares.feature:125](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/deleteShareFromShares.feature#L125)
- [coreApiShareManagementBasicToShares/deleteShareFromShares.feature:138](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/deleteShareFromShares.feature#L138)
- [coreApiShareManagementBasicToShares/deleteShareFromShares.feature:172](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/deleteShareFromShares.feature#L172)
- [coreApiShareManagementBasicToShares/deleteShareFromShares.feature:173](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/deleteShareFromShares.feature#L173)
- [coreApiShareManagementBasicToShares/deleteShareFromShares.feature:174](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/deleteShareFromShares.feature#L174)
- [coreApiShareManagementBasicToShares/deleteShareFromShares.feature:175](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/deleteShareFromShares.feature#L175)
- [coreApiShareManagementBasicToShares/deleteShareFromShares.feature:191](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/deleteShareFromShares.feature#L191)
- [coreApiShareManagementBasicToShares/deleteShareFromShares.feature:192](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/deleteShareFromShares.feature#L192)
- [coreApiShareManagementBasicToShares/deleteShareFromShares.feature:193](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/deleteShareFromShares.feature#L193)
- [coreApiShareManagementBasicToShares/deleteShareFromShares.feature:194](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/deleteShareFromShares.feature#L194)
- [coreApiShareOperationsToShares1/gettingShares.feature:188](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares1/gettingShares.feature#L188)
- [coreApiShareOperationsToShares1/gettingShares.feature:189](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares1/gettingShares.feature#L189)
- [coreApiShareManagementBasicToShares/createShareToSharesFolder.feature:326](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/createShareToSharesFolder.feature#L326)
- [coreApiShareManagementBasicToShares/createShareToSharesFolder.feature:327](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/createShareToSharesFolder.feature#L327)

#### [OCS error message for attempting to access share via share id as an unauthorized user is not informative](https://github.com/owncloud/ocis/issues/1233)

- [coreApiShareOperationsToShares1/gettingShares.feature:151](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares1/gettingShares.feature#L151)
- [coreApiShareOperationsToShares1/gettingShares.feature:152](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares1/gettingShares.feature#L152)


#### [Public link enforce permissions](https://github.com/owncloud/ocis/issues/1269)

- [coreApiSharePublicLink1/accessToPublicLinkShare.feature:12](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharePublicLink1/accessToPublicLinkShare.feature#L12)
- [coreApiSharePublicLink1/accessToPublicLinkShare.feature:31](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharePublicLink1/accessToPublicLinkShare.feature#L31)
- [coreApiSharePublicLink1/createPublicLinkShare.feature:163](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharePublicLink1/createPublicLinkShare.feature#L163)
- [coreApiSharePublicLink1/createPublicLinkShare.feature:164](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharePublicLink1/createPublicLinkShare.feature#L164)
- [coreApiSharePublicLink1/createPublicLinkShare.feature:317](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharePublicLink1/createPublicLinkShare.feature#L317)
- [coreApiSharePublicLink1/createPublicLinkShare.feature:327](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharePublicLink1/createPublicLinkShare.feature#L327)

#### [copying a folder within a public link folder to folder with same name as an already existing file overwrites the parent file](https://github.com/owncloud/ocis/issues/1232)

- [coreApiSharePublicLink2/copyFromPublicLink.feature:65](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharePublicLink2/copyFromPublicLink.feature#L65)
- [coreApiSharePublicLink2/copyFromPublicLink.feature:91](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharePublicLink2/copyFromPublicLink.feature#L91)
- [coreApiSharePublicLink2/copyFromPublicLink.feature:175](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharePublicLink2/copyFromPublicLink.feature#L175)
- [coreApiSharePublicLink2/copyFromPublicLink.feature:176](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharePublicLink2/copyFromPublicLink.feature#L176)
- [coreApiSharePublicLink2/copyFromPublicLink.feature:191](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharePublicLink2/copyFromPublicLink.feature#L191)
- [coreApiSharePublicLink2/copyFromPublicLink.feature:192](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharePublicLink2/copyFromPublicLink.feature#L192)

#### [Upload-only shares must not overwrite but create a separate file](https://github.com/owncloud/ocis/issues/1267)

- [coreApiSharePublicLink3/uploadToPublicLinkShare.feature:13](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharePublicLink3/uploadToPublicLinkShare.feature#L13)
- [coreApiSharePublicLink3/uploadToPublicLinkShare.feature:114](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharePublicLink3/uploadToPublicLinkShare.feature#L114)

#### [Set quota over settings](https://github.com/owncloud/ocis/issues/1290)
_requires a [CS3 user provisioning api that can update the quota for a user](https://github.com/cs3org/cs3apis/pull/95#issuecomment-772780683)_

- [coreApiSharePublicLink3/uploadToPublicLinkShare.feature:87](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharePublicLink3/uploadToPublicLinkShare.feature#L87)
- [coreApiSharePublicLink3/uploadToPublicLinkShare.feature:96](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharePublicLink3/uploadToPublicLinkShare.feature#L96)

#### [deleting a file inside a received shared folder is moved to the trash-bin of the sharer not the receiver](https://github.com/owncloud/ocis/issues/1124)

- [coreApiTrashbin/trashbinSharingToShares.feature:46](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinSharingToShares.feature#L46)
- [coreApiTrashbin/trashbinSharingToShares.feature:73](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinSharingToShares.feature#L73)
- [coreApiTrashbin/trashbinSharingToShares.feature:100](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinSharingToShares.feature#L100)
- [coreApiTrashbin/trashbinSharingToShares.feature:128](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinSharingToShares.feature#L128)
- [coreApiTrashbin/trashbinSharingToShares.feature:156](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinSharingToShares.feature#L156)
- [coreApiTrashbin/trashbinSharingToShares.feature:184](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinSharingToShares.feature#L184)

#### [Folder overwrite on shared files doesn't works correctly on copying file](https://github.com/owncloud/ocis/issues/2183)
- [coreApiWebdavProperties1/copyFile.feature:464](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/copyFile.feature#L464)
- [coreApiWebdavProperties1/copyFile.feature:546](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/copyFile.feature#L546)

#### [cannot get share-types webdav property](https://github.com/owncloud/ocis/issues/567)
- [coreApiWebdavProperties2/getFileProperties.feature:237](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties2/getFileProperties.feature#L237)
- [coreApiWebdavProperties2/getFileProperties.feature:238](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties2/getFileProperties.feature#L238)

#### [Scoped links](https://github.com/owncloud/ocis/issues/2809)
#### [oc:privatelink property not returned in webdav responses](https://github.com/owncloud/product/issues/262)
- [coreApiWebdavProperties2/getFileProperties.feature:293](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties2/getFileProperties.feature#L293)
- [coreApiWebdavProperties2/getFileProperties.feature:294](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties2/getFileProperties.feature#L294)

#### [changing user quota gives ocs status 103 / Cannot set quota](https://github.com/owncloud/product/issues/247)
_requires a [CS3 user provisioning api that can update the quota for a user](https://github.com/cs3org/cs3apis/pull/95#issuecomment-772780683)_
- [coreApiShareOperationsToShares2/uploadToShare.feature:210](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/uploadToShare.feature#L210)
- [coreApiShareOperationsToShares2/uploadToShare.feature:211](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/uploadToShare.feature#L211)

#### [not possible to move file into a received folder](https://github.com/owncloud/ocis/issues/764)

- [coreApiShareOperationsToShares1/changingFilesShare.feature:26](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares1/changingFilesShare.feature#L26)
- [coreApiShareOperationsToShares1/changingFilesShare.feature:27](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares1/changingFilesShare.feature#L27)
- [coreApiShareOperationsToShares1/changingFilesShare.feature:70](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares1/changingFilesShare.feature#L70)
- [coreApiShareOperationsToShares1/changingFilesShare.feature:71](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares1/changingFilesShare.feature#L71)
- [coreApiShareOperationsToShares1/changingFilesShare.feature:92](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares1/changingFilesShare.feature#L92)
- [coreApiShareOperationsToShares1/changingFilesShare.feature:93](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares1/changingFilesShare.feature#L93)
- [coreApiShareOperationsToShares1/changingFilesShare.feature:108](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares1/changingFilesShare.feature#L108)
- [coreApiShareOperationsToShares1/changingFilesShare.feature:109](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares1/changingFilesShare.feature#L109)
- [coreApiWebdavMove2/moveShareOnOcis.feature:29](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove2/moveShareOnOcis.feature#L29)
- [coreApiWebdavMove2/moveShareOnOcis.feature:31](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove2/moveShareOnOcis.feature#L31)
- [coreApiWebdavMove2/moveShareOnOcis.feature:74](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove2/moveShareOnOcis.feature#L74)
- [coreApiWebdavMove2/moveShareOnOcis.feature:75](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove2/moveShareOnOcis.feature#L75)
- [coreApiWebdavMove2/moveShareOnOcis.feature:97](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove2/moveShareOnOcis.feature#L97)
- [coreApiWebdavMove2/moveShareOnOcis.feature:99](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove2/moveShareOnOcis.feature#L99)
- [coreApiWebdavMove2/moveShareOnOcis.feature:148](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove2/moveShareOnOcis.feature#L148)
- [coreApiWebdavMove2/moveShareOnOcis.feature:149](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove2/moveShareOnOcis.feature#L149)
- [coreApiWebdavMove2/moveShareOnOcis.feature:168](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove2/moveShareOnOcis.feature#L168)
- [coreApiWebdavMove2/moveShareOnOcis.feature:169](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove2/moveShareOnOcis.feature#L169)
- [coreApiWebdavMove2/moveFile.feature:175](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove2/moveFile.feature#L175)
- [coreApiWebdavMove2/moveFile.feature:176](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove2/moveFile.feature#L176)

#### [OCIS-storage overwriting a file as share receiver, does not create a new file version for the sharer](https://github.com/owncloud/ocis/issues/766) //todo
- [coreApiVersions/fileVersions.feature:274](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiVersions/fileVersions.feature#L274)

#### [restoring an older version of a shared file deletes the share](https://github.com/owncloud/ocis/issues/765)
- [coreApiShareManagementToShares/acceptShares.feature:448](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementToShares/acceptShares.feature#L448)
- [coreApiVersions/fileVersions.feature:286](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiVersions/fileVersions.feature#L286)

#### [Expiration date for shares is not implemented](https://github.com/owncloud/ocis/issues/1250)
#### Expiration date of user shares
- [coreApiShareReshareToShares3/reShareWithExpiryDate.feature:35](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares3/reShareWithExpiryDate.feature#L35)
- [coreApiShareReshareToShares3/reShareWithExpiryDate.feature:36](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares3/reShareWithExpiryDate.feature#L36)

#### Expiration date of group shares
- [coreApiShareReshareToShares3/reShareWithExpiryDate.feature:60](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares3/reShareWithExpiryDate.feature#L60)
- [coreApiShareReshareToShares3/reShareWithExpiryDate.feature:61](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares3/reShareWithExpiryDate.feature#L61)
- [coreApiShareReshareToShares3/reShareWithExpiryDate.feature:82](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares3/reShareWithExpiryDate.feature#L82)
- [coreApiShareReshareToShares3/reShareWithExpiryDate.feature:83](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares3/reShareWithExpiryDate.feature#L83)


#### [Getting content of a shared file with same name returns 500](https://github.com/owncloud/ocis/issues/3880)
- [coreApiShareCreateSpecialToShares1/createShareUniqueReceivedNames.feature:15](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareCreateSpecialToShares1/createShareUniqueReceivedNames.feature#L15)

#### [Empty OCS response for a share create request using a disabled user](https://github.com/owncloud/ocis/issues/2212)
- [coreApiShareCreateSpecialToShares2/createShareWithDisabledUser.feature:21](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareCreateSpecialToShares2/createShareWithDisabledUser.feature#L21)
- [coreApiShareCreateSpecialToShares2/createShareWithDisabledUser.feature:22](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareCreateSpecialToShares2/createShareWithDisabledUser.feature#L22)
- [coreApiShareUpdateToShares/updateShare.feature:96](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareUpdateToShares/updateShare.feature#L96)
- [coreApiShareUpdateToShares/updateShare.feature:97](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareUpdateToShares/updateShare.feature#L97)
- [coreApiShareUpdateToShares/updateShare.feature:98](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareUpdateToShares/updateShare.feature#L98)
- [coreApiShareUpdateToShares/updateShare.feature:99](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareUpdateToShares/updateShare.feature#L99)
- [coreApiShareUpdateToShares/updateShare.feature:100](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareUpdateToShares/updateShare.feature#L100)
- [coreApiShareUpdateToShares/updateShare.feature:101](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareUpdateToShares/updateShare.feature#L101)
- [coreApiShareUpdateToShares/updateShare.feature:120](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareUpdateToShares/updateShare.feature#L120)
- [coreApiShareUpdateToShares/updateShare.feature:121](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareUpdateToShares/updateShare.feature#L121)
- [coreApiShareUpdateToShares/updateShare.feature:122](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareUpdateToShares/updateShare.feature#L122)
- [coreApiShareUpdateToShares/updateShare.feature:123](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareUpdateToShares/updateShare.feature#L123)
- [coreApiShareUpdateToShares/updateShare.feature:124](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareUpdateToShares/updateShare.feature#L124)
- [coreApiShareUpdateToShares/updateShare.feature:125](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareUpdateToShares/updateShare.feature#L125)

#### [Edit user share response has an "name" field](https://github.com/owncloud/ocis/issues/1225)
- [coreApiShareUpdateToShares/updateShare.feature:235](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareUpdateToShares/updateShare.feature#L235)
- [coreApiShareUpdateToShares/updateShare.feature:236](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareUpdateToShares/updateShare.feature#L236)

#### [user can access version metadata of a received share before accepting it](https://github.com/owncloud/ocis/issues/760)
- [coreApiVersions/fileVersions.feature:311](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiVersions/fileVersions.feature#L311)

#### [Share lists deleted user as 'user'](https://github.com/owncloud/ocis/issues/903)
- [coreApiShareManagementBasicToShares/createShareToSharesFolder.feature:667](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/createShareToSharesFolder.feature#L667)
- [coreApiShareManagementBasicToShares/createShareToSharesFolder.feature:668](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/createShareToSharesFolder.feature#L668)

#### [OCIS-storage overwriting a file as share receiver, does not create a new file version for the sharer](https://github.com/owncloud/ocis/issues/766) //todo
- [coreApiVersions/fileVersions.feature:431](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiVersions/fileVersions.feature#L431)

#### [deleting a share with wrong authentication returns OCS status 996 / HTTP 500](https://github.com/owncloud/ocis/issues/1229)
- [coreApiShareManagementBasicToShares/deleteShareFromShares.feature:221](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/deleteShareFromShares.feature#L221)
- [coreApiShareManagementBasicToShares/deleteShareFromShares.feature:222](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/deleteShareFromShares.feature#L222)

### User Management
User and group management features

### Other
API, search, favorites, config, capabilities, not existing endpoints, CORS and others

#### [no robots.txt available](https://github.com/owncloud/ocis/issues/1314)
- [coreApiMain/main.feature:7](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiMain/main.feature#L7) Scenario: robots.txt file should be accessible

#### [Ability to return error messages in Webdav response bodies](https://github.com/owncloud/ocis/issues/1293)
- [coreApiAuthOcs/ocsDELETEAuth.feature:10](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthOcs/ocsDELETEAuth.feature#L10) Scenario: send DELETE requests to OCS endpoints as admin with wrong password
- [coreApiAuthOcs/ocsGETAuth.feature:10](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthOcs/ocsGETAuth.feature#L10) Scenario: using OCS anonymously
- [coreApiAuthOcs/ocsGETAuth.feature:31](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthOcs/ocsGETAuth.feature#L31) Scenario: ocs config end point accessible by unauthorized users
- [coreApiAuthOcs/ocsGETAuth.feature:44](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthOcs/ocsGETAuth.feature#L44) Scenario: using OCS with non-admin basic auth
- [coreApiAuthOcs/ocsGETAuth.feature:75](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthOcs/ocsGETAuth.feature#L75) Scenario: using OCS as normal user with wrong password
- [coreApiAuthOcs/ocsGETAuth.feature:106](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthOcs/ocsGETAuth.feature#L106) Scenario:using OCS with admin basic auth
- [coreApiAuthOcs/ocsGETAuth.feature:123](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthOcs/ocsGETAuth.feature#L123) Scenario: using OCS as admin user with wrong password
- [coreApiAuthOcs/ocsPOSTAuth.feature:10](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthOcs/ocsPOSTAuth.feature#L10) Scenario: send POST requests to OCS endpoints as normal user with wrong password
- [coreApiAuthOcs/ocsPUTAuth.feature:10](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthOcs/ocsPUTAuth.feature#L10) Scenario: send PUT request to OCS endpoints as admin with wrong password
- [coreApiSharePublicLink1/createPublicLinkShare.feature:69](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharePublicLink1/createPublicLinkShare.feature#L69)
- [coreApiSharePublicLink1/createPublicLinkShare.feature:70](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharePublicLink1/createPublicLinkShare.feature#L70)

#### [sending MKCOL requests to another or non-existing user's webDav endpoints as normal user should return 404](https://github.com/owncloud/ocis/issues/5049)
_ocdav: api compatibility, return correct status code_
- [coreApiAuthWebDav/webDavDELETEAuth.feature:48](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthWebDav/webDavDELETEAuth.feature#L48) Scenario: send DELETE requests to another user's webDav endpoints as normal user
- [coreApiAuthWebDav/webDavPROPFINDAuth.feature:45](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthWebDav/webDavPROPFINDAuth.feature#L45)  Scenario: send PROPFIND requests to another user's webDav endpoints as normal user
- [coreApiAuthWebDav/webDavPROPPATCHAuth.feature:46](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthWebDav/webDavPROPPATCHAuth.feature#L46) Scenario: send PROPPATCH requests to another user's webDav endpoints as normal user
- [coreApiAuthWebDav/webDavMKCOLAuth.feature:42](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthWebDav/webDavMKCOLAuth.feature#L42) Scenario: send MKCOL requests to another user's webDav endpoints as normal user
- [coreApiAuthWebDav/webDavMKCOLAuth.feature:53](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthWebDav/webDavMKCOLAuth.feature#L53) Scenario: send MKCOL requests to another user's webDav endpoints as normal user using the spaces WebDAV API

#### [trying to lock file of another user gives http 200](https://github.com/owncloud/ocis/issues/2176)
- [coreApiAuthWebDav/webDavLOCKAuth.feature:46](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthWebDav/webDavLOCKAuth.feature#L46) Scenario: send LOCK requests to another user's webDav endpoints as normal user

#### [send (MOVE, COPY) requests to another user's webDav endpoints as normal user gives 400 instead of 403](https://github.com/owncloud/ocis/issues/3882)
_ocdav: api compatibility, return correct status code_
- [coreApiAuthWebDav/webDavMOVEAuth.feature:45](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthWebDav/webDavMOVEAuth.feature#L45) Scenario: send MOVE requests to another user's webDav endpoints as normal user
- [coreApiAuthWebDav/webDavCOPYAuth.feature:45](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthWebDav/webDavCOPYAuth.feature#L45)

#### [send POST requests to another user's webDav endpoints as normal user](https://github.com/owncloud/ocis/issues/1287)
_ocdav: api compatibility, return correct status code_
- [coreApiAuthWebDav/webDavPOSTAuth.feature:46](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthWebDav/webDavPOSTAuth.feature#L46)  Scenario: send POST requests to another user's webDav endpoints as normal user

#### [Using double slash in URL to access a folder gives 501 and other status codes](https://github.com/owncloud/ocis/issues/1667)
- [coreApiAuthWebDav/webDavSpecialURLs.feature:36](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthWebDav/webDavSpecialURLs.feature#L36)
- [coreApiAuthWebDav/webDavSpecialURLs.feature:123](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthWebDav/webDavSpecialURLs.feature#L123)
- [coreApiAuthWebDav/webDavSpecialURLs.feature:165](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthWebDav/webDavSpecialURLs.feature#L165)

#### [Difference in response content of status.php and default capabilities](https://github.com/owncloud/ocis/issues/1286)
- [coreApiCapabilities/capabilitiesWithNormalUser.feature:13](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiCapabilities/capabilitiesWithNormalUser.feature#L13)

#### [spaces endpoint does not allow REPORT requests](https://github.com/owncloud/ocis/issues/4034)
- [coreApiWebdavOperations/search.feature:42](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L42)
- [coreApiWebdavOperations/search.feature:43](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L43)
- [coreApiWebdavOperations/search.feature:64](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L64)
- [coreApiWebdavOperations/search.feature:65](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L65)
- [coreApiWebdavOperations/search.feature:87](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L87)
- [coreApiWebdavOperations/search.feature:88](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L88)
- [coreApiWebdavOperations/search.feature:102](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L102)
- [coreApiWebdavOperations/search.feature:103](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L103)
- [coreApiWebdavOperations/search.feature:126](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L126)
- [coreApiWebdavOperations/search.feature:127](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L127)
- [coreApiWebdavOperations/search.feature:150](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L150)
- [coreApiWebdavOperations/search.feature:151](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L151)
- [coreApiWebdavOperations/search.feature:175](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L175)
- [coreApiWebdavOperations/search.feature:176](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L176)
- [coreApiWebdavOperations/search.feature:208](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L208)
- [coreApiWebdavOperations/search.feature:209](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L209)
- [coreApiWebdavOperations/search.feature:240](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L240)
- [coreApiWebdavOperations/search.feature:241](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L241)
- [coreApiWebdavOperations/search.feature:265](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L265)
- [coreApiWebdavOperations/search.feature:266](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/search.feature#L266)

#### [Support for favorites](https://github.com/owncloud/ocis/issues/1228)
- [coreApiFavorites/favorites.feature:117](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favorites.feature#L117)
- [coreApiFavorites/favorites.feature:118](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favorites.feature#L118)
- [coreApiFavorites/favorites.feature:169](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favorites.feature#L169)
- [coreApiFavorites/favorites.feature:170](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favorites.feature#L170)
- [coreApiFavorites/favorites.feature:202](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favorites.feature#L202)
- [coreApiFavorites/favorites.feature:203](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favorites.feature#L203)
- [coreApiFavorites/favorites.feature:221](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favorites.feature#L221)
- [coreApiFavorites/favorites.feature:222](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favorites.feature#L222)
- [coreApiFavorites/favoritesSharingToShares.feature:67](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favoritesSharingToShares.feature#L67)
- [coreApiFavorites/favoritesSharingToShares.feature:68](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favoritesSharingToShares.feature#L68)

#### [resource inside Shares dir is not found using the spaces WebDAV API](https://github.com/owncloud/ocis/issues/2968)
- [coreApiFavorites/favoritesSharingToShares.feature:22](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favoritesSharingToShares.feature#L22)
- [coreApiFavorites/favoritesSharingToShares.feature:23](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favoritesSharingToShares.feature#L23)
- [coreApiFavorites/favoritesSharingToShares.feature:37](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favoritesSharingToShares.feature#L37)
- [coreApiFavorites/favoritesSharingToShares.feature:38](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favoritesSharingToShares.feature#L38)
- [coreApiFavorites/favoritesSharingToShares.feature:51](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favoritesSharingToShares.feature#L51)
- [coreApiFavorites/favoritesSharingToShares.feature:52](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favoritesSharingToShares.feature#L52)
- [coreApiFavorites/favoritesSharingToShares.feature:82](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favoritesSharingToShares.feature#L82)
- [coreApiFavorites/favoritesSharingToShares.feature:83](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favoritesSharingToShares.feature#L83)
- [coreApiMain/checksums.feature:189](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiMain/checksums.feature#L189)

#### [WWW-Authenticate header for unauthenticated requests is not clear](https://github.com/owncloud/ocis/issues/2285)
- [coreApiWebdavOperations/refuseAccess.feature:21](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/refuseAccess.feature#L21)
- [coreApiWebdavOperations/refuseAccess.feature:22](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/refuseAccess.feature#L22)
- [coreApiWebdavOperations/refuseAccess.feature:34](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/refuseAccess.feature#L34)
- [coreApiWebdavOperations/refuseAccess.feature:35](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/refuseAccess.feature#L35)

#### [App Passwords/Tokens for legacy WebDAV clients](https://github.com/owncloud/ocis/issues/197)
- [coreApiAuthWebDav/webDavDELETEAuth.feature:108](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthWebDav/webDavDELETEAuth.feature#L108)

#### [Sharing a same file twice to the same group](https://github.com/owncloud/ocis/issues/1710)
- [coreApiShareManagementBasicToShares/createShareToSharesFolder.feature:767](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/createShareToSharesFolder.feature#L767)
- [coreApiShareManagementBasicToShares/createShareToSharesFolder.feature:768](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/createShareToSharesFolder.feature#L768)

#### [Request to edit non-existing user by authorized admin gets unauthorized in http response](https://github.com/owncloud/ocis/issues/38423)
- [coreApiAuthOcs/ocsPUTAuth.feature:26](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthOcs/ocsPUTAuth.feature#L26)


### Won't fix
Not everything needs to be implemented for ocis. While the oc10 testsuite covers these things we are not looking at them right now.

* _The `OC-LazyOps` header is [no longer supported by the client](https://github.com/owncloud/client/pull/8398), implmenting this is not necessary for a first production release. We plan to have an uploed state machine to visualize the state of a file, see https://github.com/owncloud/ocis/issues/214_
* _Blacklisted ignored files are no longer required because ocis can handle `.htaccess` files without security implications introduced by serving user provided files with apache._

#### [Blacklist files extensions](https://github.com/owncloud/ocis/issues/2177)
- [coreApiWebdavProperties1/copyFile.feature:118](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/copyFile.feature#L118)
- [coreApiWebdavProperties1/copyFile.feature:117](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/copyFile.feature#L117)
- [coreApiWebdavProperties1/createFileFolder.feature:97](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/createFileFolder.feature#L97)
- [coreApiWebdavProperties1/createFileFolder.feature:98](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/createFileFolder.feature#L98)
- [coreApiWebdavUpload1/uploadFile.feature:180](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUpload1/uploadFile.feature#L180)
- [coreApiWebdavUpload1/uploadFile.feature:181](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUpload1/uploadFile.feature#L181)

#### [cannot set blacklisted file names](https://github.com/owncloud/product/issues/260)
- [coreApiWebdavMove1/moveFolderToBlacklistedName.feature:20](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove1/moveFolderToBlacklistedName.feature#L20)
- [coreApiWebdavMove1/moveFolderToBlacklistedName.feature:21](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove1/moveFolderToBlacklistedName.feature#L21)

#### [cannot set blacklisted file names](https://github.com/owncloud/product/issues/260)
- [coreApiWebdavMove2/moveFileToBlacklistedName.feature:18](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove2/moveFileToBlacklistedName.feature#L18)
- [coreApiWebdavMove2/moveFileToBlacklistedName.feature:19](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove2/moveFileToBlacklistedName.feature#L19)

#### [PATCH request for TUS upload with wrong checksum gives incorrect response](https://github.com/owncloud/ocis/issues/1755)
- [coreApiWebdavUploadTUS/checksums.feature:86](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L86)
- [coreApiWebdavUploadTUS/checksums.feature:87](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L87)
- [coreApiWebdavUploadTUS/checksums.feature:88](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L88)
- [coreApiWebdavUploadTUS/checksums.feature:89](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L89)
- [coreApiWebdavUploadTUS/checksums.feature:175](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L175)
- [coreApiWebdavUploadTUS/checksums.feature:176](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L176)
- [coreApiWebdavUploadTUS/checksums.feature:228](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L228)
- [coreApiWebdavUploadTUS/checksums.feature:229](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L229)
- [coreApiWebdavUploadTUS/checksums.feature:230](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L230)
- [coreApiWebdavUploadTUS/checksums.feature:231](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L231)
- [coreApiWebdavUploadTUS/checksums.feature:284](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L284)
- [coreApiWebdavUploadTUS/checksums.feature:285](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L285)
- [coreApiWebdavUploadTUS/checksums.feature:286](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L286)
- [coreApiWebdavUploadTUS/checksums.feature:287](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L287)
- [coreApiWebdavUploadTUS/optionsRequest.feature:10](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/optionsRequest.feature#L10)
- [coreApiWebdavUploadTUS/optionsRequest.feature:25](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/optionsRequest.feature#L25)
- [coreApiWebdavUploadTUS/optionsRequest.feature:40](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/optionsRequest.feature#L40)
- [coreApiWebdavUploadTUS/optionsRequest.feature:55](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/optionsRequest.feature#L55)
- [coreApiWebdavUploadTUS/uploadToShare.feature:175](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/uploadToShare.feature#L175)
- [coreApiWebdavUploadTUS/uploadToShare.feature:176](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/uploadToShare.feature#L176)
- [coreApiWebdavUploadTUS/uploadToShare.feature:194](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/uploadToShare.feature#L194)
- [coreApiWebdavUploadTUS/uploadToShare.feature:195](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/uploadToShare.feature#L195)
- [coreApiWebdavUploadTUS/uploadToShare.feature:213](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/uploadToShare.feature#L213)
- [coreApiWebdavUploadTUS/uploadToShare.feature:214](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/uploadToShare.feature#L214)
- [coreApiWebdavUploadTUS/uploadToShare.feature:252](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/uploadToShare.feature#L252)
- [coreApiWebdavUploadTUS/uploadToShare.feature:253](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/uploadToShare.feature#L253)
- [coreApiWebdavUploadTUS/uploadToShare.feature:294](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/uploadToShare.feature#L294)
- [coreApiWebdavUploadTUS/uploadToShare.feature:295](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/uploadToShare.feature#L295)

#### [Share inaccessible if folder with same name was deleted and recreated](https://github.com/owncloud/ocis/issues/1787)
- [coreApiShareReshareToShares1/reShare.feature:267](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares1/reShare.feature#L267)
- [coreApiShareReshareToShares1/reShare.feature:268](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares1/reShare.feature#L268)
- [coreApiShareReshareToShares1/reShare.feature:285](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares1/reShare.feature#L285)
- [coreApiShareReshareToShares1/reShare.feature:286](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares1/reShare.feature#L286)
- [coreApiShareReshareToShares1/reShare.feature:303](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares1/reShare.feature#L303)
- [coreApiShareReshareToShares1/reShare.feature:304](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares1/reShare.feature#L304)

#### [incorrect ocs(v2) status value when getting info of share that does not exist should be 404, gives 998](https://github.com/owncloud/product/issues/250)
_ocs: api compatibility, return correct status code_
- [coreApiShareOperationsToShares2/shareAccessByID.feature:48](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L48)
- [coreApiShareOperationsToShares2/shareAccessByID.feature:49](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L49)
- [coreApiShareOperationsToShares2/shareAccessByID.feature:50](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L50)
- [coreApiShareOperationsToShares2/shareAccessByID.feature:51](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L51)
- [coreApiShareOperationsToShares2/shareAccessByID.feature:52](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L52)
- [coreApiShareOperationsToShares2/shareAccessByID.feature:53](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L53)
- [coreApiShareOperationsToShares2/shareAccessByID.feature:54](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L54)
- [coreApiShareOperationsToShares2/shareAccessByID.feature:55](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L55)

#### [Trying to accept a share with invalid ID gives incorrect OCS and HTTP status](https://github.com/owncloud/ocis/issues/2111)
- [coreApiShareOperationsToShares2/shareAccessByID.feature:84](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L84)
- [coreApiShareOperationsToShares2/shareAccessByID.feature:85](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L85)
- [coreApiShareOperationsToShares2/shareAccessByID.feature:86](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L86)
- [coreApiShareOperationsToShares2/shareAccessByID.feature:87](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L87)
- [coreApiShareOperationsToShares2/shareAccessByID.feature:88](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L88)
- [coreApiShareOperationsToShares2/shareAccessByID.feature:89](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L89)
- [coreApiShareOperationsToShares2/shareAccessByID.feature:90](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L90)
- [coreApiShareOperationsToShares2/shareAccessByID.feature:91](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L91)
- [coreApiShareOperationsToShares2/shareAccessByID.feature:102](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L102)
- [coreApiShareOperationsToShares2/shareAccessByID.feature:103](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L103)
- [coreApiShareOperationsToShares2/shareAccessByID.feature:133](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L133)
- [coreApiShareOperationsToShares2/shareAccessByID.feature:134](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L134)
- [coreApiShareOperationsToShares2/shareAccessByID.feature:135](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L135)
- [coreApiShareOperationsToShares2/shareAccessByID.feature:136](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L136)
- [coreApiShareOperationsToShares2/shareAccessByID.feature:137](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L137)
- [coreApiShareOperationsToShares2/shareAccessByID.feature:138](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L138)
- [coreApiShareOperationsToShares2/shareAccessByID.feature:139](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L139)
- [coreApiShareOperationsToShares2/shareAccessByID.feature:140](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L140)
- [coreApiShareOperationsToShares2/shareAccessByID.feature:151](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L151)
- [coreApiShareOperationsToShares2/shareAccessByID.feature:152](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L152)

#### [[OC-storage] share-types field empty for shared file folder in webdav response](https://github.com/owncloud/ocis/issues/2144)
- [coreApiWebdavProperties2/getFileProperties.feature:214](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties2/getFileProperties.feature#L214)
- [coreApiWebdavProperties2/getFileProperties.feature:215](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties2/getFileProperties.feature#L215)

#### [Different share permissions provides varying roles in oc10 and ocis](https://github.com/owncloud/ocis/issues/1277)
- [coreApiWebdavProperties2/getFileProperties.feature:274](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties2/getFileProperties.feature#L274)
- [coreApiWebdavProperties2/getFileProperties.feature:275](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties2/getFileProperties.feature#L275)

#### [Cannot move folder/file from one received share to another](https://github.com/owncloud/ocis/issues/2442)
- [coreApiShareUpdateToShares/updateShare.feature:128](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareUpdateToShares/updateShare.feature#L128)
- [coreApiShareUpdateToShares/updateShare.feature:160](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareUpdateToShares/updateShare.feature#L160)
- [coreApiShareManagementToShares/mergeShare.feature:131](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementToShares/mergeShare.feature#L131)

#### [Sharing folder and sub-folder with same user but different permission,the permission of sub-folder is not obeyed ](https://github.com/owncloud/ocis/issues/2440)
- [coreApiShareCreateSpecialToShares2/createShareReceivedInMultipleWays.feature:221](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareCreateSpecialToShares2/createShareReceivedInMultipleWays.feature#L221)
- [coreApiShareCreateSpecialToShares2/createShareReceivedInMultipleWays.feature:252](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareCreateSpecialToShares2/createShareReceivedInMultipleWays.feature#L252)
- [coreApiShareCreateSpecialToShares2/createShareReceivedInMultipleWays.feature:351](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareCreateSpecialToShares2/createShareReceivedInMultipleWays.feature#L351)
- [coreApiShareCreateSpecialToShares2/createShareReceivedInMultipleWays.feature:382](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareCreateSpecialToShares2/createShareReceivedInMultipleWays.feature#L382)

#### [copying the file inside Shares folder returns 404](https://github.com/owncloud/ocis/issues/3874)
- [coreApiWebdavProperties1/copyFile.feature:408](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/copyFile.feature#L408)
- [coreApiWebdavProperties1/copyFile.feature:407](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/copyFile.feature#L407)
- [coreApiWebdavProperties1/copyFile.feature:434](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/copyFile.feature#L434)
- [coreApiWebdavProperties1/copyFile.feature:433](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/copyFile.feature#L433)

#### [Shares to deleted group listed in the response](https://github.com/owncloud/ocis/issues/2441)
- [coreApiShareManagementBasicToShares/createShareToSharesFolder.feature:529](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/createShareToSharesFolder.feature#L529)
- [coreApiShareManagementBasicToShares/createShareToSharesFolder.feature:532](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/createShareToSharesFolder.feature#L532)

### [Share path in the response is different between share states](https://github.com/owncloud/ocis/issues/2540)
- [coreApiShareManagementToShares/acceptShares.feature:28](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementToShares/acceptShares.feature#L28)
- [coreApiShareManagementToShares/acceptShares.feature:62](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementToShares/acceptShares.feature#L62)
- [coreApiShareManagementToShares/acceptShares.feature:134](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementToShares/acceptShares.feature#L134)
- [coreApiShareManagementToShares/acceptShares.feature:155](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementToShares/acceptShares.feature#L155)
- [coreApiShareManagementToShares/acceptShares.feature:183](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementToShares/acceptShares.feature#L183)
- [coreApiShareManagementToShares/acceptShares.feature:228](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementToShares/acceptShares.feature#L228)
- [coreApiShareManagementToShares/acceptShares.feature:438](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementToShares/acceptShares.feature#L438)
- [coreApiShareOperationsToShares2/shareAccessByID.feature:121](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L121)
- [coreApiShareOperationsToShares2/shareAccessByID.feature:122](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L122)
- [coreApiShareManagementToShares/acceptShares.feature:208](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementToShares/acceptShares.feature#L208)

### [Content-type is not multipart/byteranges when downloading file with Range Header](https://github.com/owncloud/ocis/issues/2677)
- [coreApiWebdavOperations/downloadFile.feature:183](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/downloadFile.feature#L183)
- [coreApiWebdavOperations/downloadFile.feature:184](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/downloadFile.feature#L184)

#### [Creating a new folder which is a substring of Shares leads to Unknown Error](https://github.com/owncloud/ocis/issues/3033)
- [coreApiWebdavProperties1/createFileFolderWhenSharesExist.feature:26](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/createFileFolderWhenSharesExist.feature#L26)
- [coreApiWebdavProperties1/createFileFolderWhenSharesExist.feature:29](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/createFileFolderWhenSharesExist.feature#L29)
- [coreApiWebdavProperties1/createFileFolderWhenSharesExist.feature:42](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/createFileFolderWhenSharesExist.feature#L42)
- [coreApiWebdavProperties1/createFileFolderWhenSharesExist.feature:45](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/createFileFolderWhenSharesExist.feature#L45)

#### [moveShareInsideAnotherShare behaves differently on oCIS than oC10](https://github.com/owncloud/ocis/issues/3047)
- [coreApiShareManagementToShares/moveShareInsideAnotherShare.feature:22](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementToShares/moveShareInsideAnotherShare.feature#L22)
- [coreApiShareManagementToShares/moveShareInsideAnotherShare.feature:42](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementToShares/moveShareInsideAnotherShare.feature#L42)
- [coreApiShareManagementToShares/moveShareInsideAnotherShare.feature:56](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementToShares/moveShareInsideAnotherShare.feature#L56)

#### [resource path is included in the returned error message](https://github.com/owncloud/ocis/issues/3344)
- [coreApiWebdavProperties2/getFileProperties.feature:310](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties2/getFileProperties.feature#L310)

#### [OCS status code zero](https://github.com/owncloud/ocis/issues/3621)
- [coreApiShareManagementToShares/moveReceivedShare.feature:13](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementToShares/moveReceivedShare.feature#L13)

#### [HTTP status code differ while listing the contents of another user's trash bin](https://github.com/owncloud/ocis/issues/3561)
- [coreApiTrashbin/trashbinFilesFolders.feature:249](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinFilesFolders.feature#L249)

#### [Cannot disable the dav propfind depth infinity for resources](https://github.com/owncloud/ocis/issues/3720)
- [coreApiWebdavOperations/listFiles.feature:354](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/listFiles.feature#L354)
- [coreApiWebdavOperations/listFiles.feature:355](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/listFiles.feature#L355)
- [coreApiWebdavOperations/listFiles.feature:374](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/listFiles.feature#L374)
- [coreApiWebdavOperations/listFiles.feature:393](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/listFiles.feature#L393)
- [coreApiWebdavOperations/listFiles.feature:394](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/listFiles.feature#L394)

#### [trash-bin propfind responses are wrong in reva master](https://github.com/cs3org/reva/issues/2861)
- [coreApiTrashbin/trashbinDelete.feature:29](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinDelete.feature#L29)
- [coreApiTrashbin/trashbinDelete.feature:30](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinDelete.feature#L30)
- [coreApiTrashbin/trashbinDelete.feature:53](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinDelete.feature#L53)
- [coreApiTrashbin/trashbinDelete.feature:80](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinDelete.feature#L80)
- [coreApiTrashbin/trashbinDelete.feature:123](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinDelete.feature#L123)
- [coreApiTrashbin/trashbinDelete.feature:146](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinDelete.feature#L146)
- [coreApiTrashbin/trashbinDelete.feature:171](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinDelete.feature#L171)
- [coreApiTrashbin/trashbinDelete.feature:196](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinDelete.feature#L196)
- [coreApiTrashbin/trashbinDelete.feature:233](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinDelete.feature#L233)
- [coreApiTrashbin/trashbinDelete.feature:270](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinDelete.feature#L270)
- [coreApiTrashbin/trashbinDelete.feature:319](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinDelete.feature#L319)
- [coreApiTrashbin/trashbinFilesFolders.feature:20](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinFilesFolders.feature#L20)
- [coreApiTrashbin/trashbinFilesFolders.feature:36](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinFilesFolders.feature#L36)
- [coreApiTrashbin/trashbinFilesFolders.feature:55](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinFilesFolders.feature#L55)
- [coreApiTrashbin/trashbinFilesFolders.feature:76](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinFilesFolders.feature#L76)
- [coreApiTrashbin/trashbinFilesFolders.feature:95](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinFilesFolders.feature#L95)
- [coreApiTrashbin/trashbinFilesFolders.feature:131](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinFilesFolders.feature#L131)
- [coreApiTrashbin/trashbinFilesFolders.feature:154](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinFilesFolders.feature#L154)
- [coreApiTrashbin/trashbinFilesFolders.feature:287](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinFilesFolders.feature#L287)
- [coreApiTrashbin/trashbinFilesFolders.feature:305](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinFilesFolders.feature#L305)
- [coreApiTrashbin/trashbinFilesFolders.feature:306](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinFilesFolders.feature#L306)
- [coreApiTrashbin/trashbinFilesFolders.feature:307](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinFilesFolders.feature#L307)
- [coreApiTrashbin/trashbinFilesFolders.feature:326](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinFilesFolders.feature#L326)
- [coreApiTrashbin/trashbinFilesFolders.feature:346](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinFilesFolders.feature#L346)
- [coreApiTrashbin/trashbinFilesFolders.feature:400](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinFilesFolders.feature#L400)
- [coreApiTrashbin/trashbinFilesFolders.feature:437](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinFilesFolders.feature#L437)
- [coreApiTrashbin/trashbinSharingToShares.feature:24](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinSharingToShares.feature#L24)
- [coreApiTrashbin/trashbinSharingToShares.feature:207](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinSharingToShares.feature#L207)
- [coreApiTrashbin/trashbinSharingToShares.feature:231](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinSharingToShares.feature#L231)
- [coreApiTrashbinRestore/trashbinRestore.feature:34](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L34)
- [coreApiTrashbinRestore/trashbinRestore.feature:35](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L35)
- [coreApiTrashbinRestore/trashbinRestore.feature:50](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L50)
- [coreApiTrashbinRestore/trashbinRestore.feature:51](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L51)
- [coreApiTrashbinRestore/trashbinRestore.feature:68](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L68)
- [coreApiTrashbinRestore/trashbinRestore.feature:69](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L69)
- [coreApiTrashbinRestore/trashbinRestore.feature:88](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L88)
- [coreApiTrashbinRestore/trashbinRestore.feature:89](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L89)
- [coreApiTrashbinRestore/trashbinRestore.feature:90](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L90)
- [coreApiTrashbinRestore/trashbinRestore.feature:91](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L91)
- [coreApiTrashbinRestore/trashbinRestore.feature:92](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L92)
- [coreApiTrashbinRestore/trashbinRestore.feature:93](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L93)
- [coreApiTrashbinRestore/trashbinRestore.feature:108](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L108)
- [coreApiTrashbinRestore/trashbinRestore.feature:109](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L109)
- [coreApiTrashbinRestore/trashbinRestore.feature:110](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L110)
- [coreApiTrashbinRestore/trashbinRestore.feature:111](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L111)
- [coreApiTrashbinRestore/trashbinRestore.feature:130](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L130)
- [coreApiTrashbinRestore/trashbinRestore.feature:131](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L131)
- [coreApiTrashbinRestore/trashbinRestore.feature:145](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L145)
- [coreApiTrashbinRestore/trashbinRestore.feature:146](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L146)
- [coreApiTrashbinRestore/trashbinRestore.feature:160](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L160)
- [coreApiTrashbinRestore/trashbinRestore.feature:161](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L161)
- [coreApiTrashbinRestore/trashbinRestore.feature:175](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L175)
- [coreApiTrashbinRestore/trashbinRestore.feature:176](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L176)
- [coreApiTrashbinRestore/trashbinRestore.feature:192](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L192)
- [coreApiTrashbinRestore/trashbinRestore.feature:193](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L193)
- [coreApiTrashbinRestore/trashbinRestore.feature:194](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L194)
- [coreApiTrashbinRestore/trashbinRestore.feature:195](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L195)
- [coreApiTrashbinRestore/trashbinRestore.feature:190](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L190)
- [coreApiTrashbinRestore/trashbinRestore.feature:191](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L191)
- [coreApiTrashbinRestore/trashbinRestore.feature:212](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L212)
- [coreApiTrashbinRestore/trashbinRestore.feature:213](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L213)
- [coreApiTrashbinRestore/trashbinRestore.feature:230](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L230)
- [coreApiTrashbinRestore/trashbinRestore.feature:231](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L231)
- [coreApiTrashbinRestore/trashbinRestore.feature:250](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L250)
- [coreApiTrashbinRestore/trashbinRestore.feature:251](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L251)
- [coreApiTrashbinRestore/trashbinRestore.feature:270](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L270)
- [coreApiTrashbinRestore/trashbinRestore.feature:271](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L271)
- [coreApiTrashbinRestore/trashbinRestore.feature:304](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L304)
- [coreApiTrashbinRestore/trashbinRestore.feature:305](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L305)
- [coreApiTrashbinRestore/trashbinRestore.feature:343](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L343)
- [coreApiTrashbinRestore/trashbinRestore.feature:344](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L344)
- [coreApiTrashbinRestore/trashbinRestore.feature:382](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L382)
- [coreApiTrashbinRestore/trashbinRestore.feature:383](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L383)
- [coreApiTrashbinRestore/trashbinRestore.feature:400](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L400)
- [coreApiTrashbinRestore/trashbinRestore.feature:401](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L401)
- [coreApiTrashbinRestore/trashbinRestore.feature:419](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L419)
- [coreApiTrashbinRestore/trashbinRestore.feature:420](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L420)
- [coreApiTrashbinRestore/trashbinRestore.feature:443](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L443)
- [coreApiTrashbinRestore/trashbinRestore.feature:444](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L444)
- [coreApiTrashbinRestore/trashbinRestore.feature:462](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L462)
- [coreApiTrashbinRestore/trashbinRestore.feature:463](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L463)
- [coreApiTrashbinRestore/trashbinRestore.feature:477](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L477)
- [coreApiTrashbinRestore/trashbinRestore.feature:478](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L478)
- [coreApiTrashbinRestore/trashbinRestore.feature:531](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L531)
- [coreApiTrashbinRestore/trashbinRestore.feature:532](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L532)
- [coreApiWebdavEtagPropagation2/restoreFromTrash.feature:28](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavEtagPropagation2/restoreFromTrash.feature#L28)
- [coreApiWebdavEtagPropagation2/restoreFromTrash.feature:29](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavEtagPropagation2/restoreFromTrash.feature#L29)
- [coreApiWebdavEtagPropagation2/restoreFromTrash.feature:51](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavEtagPropagation2/restoreFromTrash.feature#L51)
- [coreApiWebdavEtagPropagation2/restoreFromTrash.feature:52](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavEtagPropagation2/restoreFromTrash.feature#L52)
- [coreApiWebdavEtagPropagation2/restoreFromTrash.feature:72](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavEtagPropagation2/restoreFromTrash.feature#L72)
- [coreApiWebdavEtagPropagation2/restoreFromTrash.feature:73](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavEtagPropagation2/restoreFromTrash.feature#L73)
- [coreApiWebdavEtagPropagation2/restoreFromTrash.feature:95](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavEtagPropagation2/restoreFromTrash.feature#L95)
- [coreApiWebdavEtagPropagation2/restoreFromTrash.feature:96](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavEtagPropagation2/restoreFromTrash.feature#L96)
- [coreApiVersions/fileVersions.feature:231](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiVersions/fileVersions.feature#L231)

#### [`meta` requests have empty responses with master branch](https://github.com/cs3org/reva/issues/2897)
- [coreApiVersions/fileVersions.feature:195](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiVersions/fileVersions.feature#L195)
- [coreApiVersions/fileVersions.feature:201](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiVersions/fileVersions.feature#L201)
- [coreApiVersions/fileVersions.feature:208](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiVersions/fileVersions.feature#L208)
- [coreApiVersions/fileVersions.feature:216](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiVersions/fileVersions.feature#L216)
- [coreApiVersions/fileVersions.feature:229](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiVersions/fileVersions.feature#L229)
- [coreApiVersions/fileVersions.feature:230](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiVersions/fileVersions.feature#L230)
- [coreApiVersions/fileVersions.feature:232](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiVersions/fileVersions.feature#L232)
- [coreApiVersions/fileVersions.feature:235](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiVersions/fileVersions.feature#L235)

#### [WebDAV MOVE with body returns 400 rather than 415](https://github.com/cs3org/reva/issues/3119)

- [coreApiAuthWebDav/webDavMOVEAuth.feature:105](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthWebDav/webDavMOVEAuth.feature#L105)

#### [reShareUpdate API tests failing in reva](https://github.com/cs3org/reva/issues/2916)
- [coreApiShareReshareToShares3/reShareUpdate.feature:153](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares3/reShareUpdate.feature#L153)
- [coreApiShareReshareToShares3/reShareUpdate.feature:154](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares3/reShareUpdate.feature#L154)

#### [coreApiShareOperationsToShares1/gettingShares.feature:28 fails in CI](https://github.com/cs3org/reva/issues/2926)

- [coreApiShareOperationsToShares1/gettingShares.feature:39](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares1/gettingShares.feature#L39)
- [coreApiShareOperationsToShares1/gettingShares.feature:40](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares1/gettingShares.feature#L40)

#### [These tests pass in ocis and reva egde but fail in master with `file_target has unexpected value '/home'`](https://github.com/owncloud/ocis/issues/2113)
- [coreApiSharePublicLink1/createPublicLinkShare.feature:280](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharePublicLink1/createPublicLinkShare.feature#L280)
- [coreApiSharePublicLink1/createPublicLinkShare.feature:281](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharePublicLink1/createPublicLinkShare.feature#L281)

#### [valid WebDAV (DELETE, COPY or MOVE) requests with body must exit with 415](https://github.com/owncloud/ocis/issues/4332)
- [coreApiAuthWebDav/webDavCOPYAuth.feature:105](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthWebDav/webDavCOPYAuth.feature#L105)

#### [PROPFIND on (password protected) public link returns invalid XML](https://github.com/owncloud/ocis/issues/39707)
The problem has been fixed in reva edge branch but not in reva master
- [coreApiWebdavOperations/propfind.feature:64](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/propfind.feature#L64)
- [coreApiWebdavOperations/propfind.feature:76](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/propfind.feature#L76)

#### [Updating the role of a public link to internal gives returns 400]
- [coreApiSharePublicLink3/updatePublicLinkShare.feature:483](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharePublicLink3/updatePublicLinkShare.feature#L483)
- [coreApiSharePublicLink3/updatePublicLinkShare.feature:480](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharePublicLink3/updatePublicLinkShare.feature#L480)
- [coreApiSharePublicLink3/updatePublicLinkShare.feature:481](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharePublicLink3/updatePublicLinkShare.feature#L481)
- [coreApiSharePublicLink3/updatePublicLinkShare.feature:482](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharePublicLink3/updatePublicLinkShare.feature#L482)

#### [Default capabilities for normal user and admin user not same as in oC-core](https://github.com/owncloud/ocis/issues/1285)
- [coreApiCapabilities/capabilities.feature:10](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiCapabilities/capabilities.feature#L10)
- [coreApiCapabilities/capabilities.feature:135](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiCapabilities/capabilities.feature#L135)
- [coreApiCapabilities/capabilities.feature:174](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiCapabilities/capabilities.feature#L174)

#### [Sharing of project space root via public link does no longer work](https://github.com/owncloud/ocis/issues/6278)
- [coreApiShareCreateSpecialToShares2/createShareDefaultFolderForReceivedShares.feature:23](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareCreateSpecialToShares2/createShareDefaultFolderForReceivedShares.feature#L23)
- [coreApiShareCreateSpecialToShares2/createShareDefaultFolderForReceivedShares.feature:24](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareCreateSpecialToShares2/createShareDefaultFolderForReceivedShares.feature#L24)

Note: always have an empty line at the end of this file.
The bash script that processes this file may not process a scenario reference on the last line.
