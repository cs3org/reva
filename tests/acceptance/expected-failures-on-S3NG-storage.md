## Scenarios from OCIS API tests that are expected to fail with OCIS storage

The expected failures in this file are from features in the owncloud/ocis repo.

### File
Basic file management like up and download, move, copy, properties, quota, trash, versions and chunking.

#### [invalid webdav responses for unauthorized requests.](https://github.com/owncloud/product/issues/273)
- [coreApiTrashbin/trashbinFilesFolders.feature:282](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinFilesFolders.feature#L282)
- [coreApiTrashbin/trashbinFilesFolders.feature:301](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinFilesFolders.feature#L301)

### [Downloading the older version of shared file gives 404](https://github.com/owncloud/ocis/issues/3868)
- [coreApiVersions/fileVersions.feature:160](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiVersions/fileVersions.feature#L160)
- [coreApiVersions/fileVersions.feature:178](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiVersions/fileVersions.feature#L178)
- [coreApiVersions/fileVersions.feature:510](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiVersions/fileVersions.feature#L510)

#### [file versions do not report the version author](https://github.com/owncloud/ocis/issues/2914)
- [coreApiVersions/fileVersionAuthor.feature:13](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiVersions/fileVersionAuthor.feature#L13)
- [coreApiVersions/fileVersionAuthor.feature:44](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiVersions/fileVersionAuthor.feature#L44)
- [coreApiVersions/fileVersionAuthor.feature:71](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiVersions/fileVersionAuthor.feature#L71)
- [coreApiVersions/fileVersionAuthor.feature:97](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiVersions/fileVersionAuthor.feature#L97)
- [coreApiVersions/fileVersionAuthor.feature:130](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiVersions/fileVersionAuthor.feature#L130)
- [coreApiVersions/fileVersionAuthor.feature:157](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiVersions/fileVersionAuthor.feature#L157)
- [coreApiVersions/fileVersionAuthor.feature:188](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiVersions/fileVersionAuthor.feature#L188)
- [coreApiVersions/fileVersionAuthor.feature:223](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiVersions/fileVersionAuthor.feature#L223)
- [coreApiVersions/fileVersionAuthor.feature:275](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiVersions/fileVersionAuthor.feature#L275)
- [coreApiVersions/fileVersionAuthor.feature:324](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiVersions/fileVersionAuthor.feature#L324)
- [coreApiVersions/fileVersionAuthor.feature:345](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiVersions/fileVersionAuthor.feature#L345)
- [coreApiVersions/fileVersionAuthor.feature:365](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiVersions/fileVersionAuthor.feature#L365)
- [coreApiVersions/fileVersionAuthor.feature:390](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiVersions/fileVersionAuthor.feature#L390)

#### [Getting information about a folder overwritten by a file gives 500 error instead of 404](https://github.com/owncloud/ocis/issues/1239)
- [coreApiWebdavProperties1/copyFile.feature:274](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/copyFile.feature#L274)
- [coreApiWebdavProperties1/copyFile.feature:273](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/copyFile.feature#L273)
- [coreApiWebdavProperties1/copyFile.feature:292](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/copyFile.feature#L292)
- [coreApiWebdavProperties1/copyFile.feature:291](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/copyFile.feature#L291)
- [coreApiWebdavProperties1/copyFile.feature:315](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/copyFile.feature#L315)
- [coreApiWebdavProperties1/copyFile.feature:314](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/copyFile.feature#L314)
- [coreApiWebdavProperties1/copyFile.feature:340](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/copyFile.feature#L340)
- [coreApiWebdavProperties1/copyFile.feature:339](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/copyFile.feature#L339)
- [coreApiWebdavProperties1/copyFile.feature:364](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/copyFile.feature#L364)
- [coreApiWebdavProperties1/copyFile.feature:363](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/copyFile.feature#L363)
- [coreApiWebdavProperties1/copyFile.feature:388](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/copyFile.feature#L388)
- [coreApiWebdavProperties1/copyFile.feature:387](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/copyFile.feature#L387)
- [coreApiWebdavProperties1/copyFile.feature:467](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/copyFile.feature#L467)
- [coreApiWebdavProperties1/copyFile.feature:495](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/copyFile.feature#L495)
- [coreApiWebdavProperties1/copyFile.feature:494](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/copyFile.feature#L494)
- [coreApiWebdavProperties1/copyFile.feature:522](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/copyFile.feature#L522)
- [coreApiWebdavProperties1/copyFile.feature:521](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/copyFile.feature#L521)
- [coreApiWebdavProperties1/copyFile.feature:549](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/copyFile.feature#L549)

#### [Custom dav properties with namespaces are rendered incorrectly](https://github.com/owncloud/ocis/issues/2140)
_ocdav: double check the webdav property parsing when custom namespaces are used_
- [coreApiWebdavProperties1/setFileProperties.feature:37](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/setFileProperties.feature#L37)
- [coreApiWebdavProperties1/setFileProperties.feature:38](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/setFileProperties.feature#L38)
- [coreApiWebdavProperties1/setFileProperties.feature:78](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/setFileProperties.feature#L78)
- [coreApiWebdavProperties1/setFileProperties.feature:79](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/setFileProperties.feature#L79)

#### [Cannot set custom webDav properties](https://github.com/owncloud/product/issues/264)
- [coreApiWebdavProperties2/getFileProperties.feature:341](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties2/getFileProperties.feature#L341)
- [coreApiWebdavProperties2/getFileProperties.feature:346](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties2/getFileProperties.feature#L346)
- [coreApiWebdavProperties2/getFileProperties.feature:381](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties2/getFileProperties.feature#L381)
- [coreApiWebdavProperties2/getFileProperties.feature:386](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties2/getFileProperties.feature#L386)

### Sync
Synchronization features like etag propagation, setting mtime and locking files

#### [Uploading an old method chunked file with checksum should fail using new DAV path](https://github.com/owncloud/ocis/issues/2323)
- [coreApiMain/checksums.feature:266](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiMain/checksums.feature#L266)

#### [Webdav LOCK operations](https://github.com/owncloud/ocis/issues/1284)
- [coreApiWebdavLocks/exclusiveLocks.feature:22](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks/exclusiveLocks.feature#L22)
- [coreApiWebdavLocks/exclusiveLocks.feature:19](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks/exclusiveLocks.feature#L19)
- [coreApiWebdavLocks/exclusiveLocks.feature:20](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks/exclusiveLocks.feature#L20)
- [coreApiWebdavLocks/exclusiveLocks.feature:21](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks/exclusiveLocks.feature#L21)
- [coreApiWebdavLocks/exclusiveLocks.feature:49](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks/exclusiveLocks.feature#L49)
- [coreApiWebdavLocks/exclusiveLocks.feature:46](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks/exclusiveLocks.feature#L46)
- [coreApiWebdavLocks/exclusiveLocks.feature:47](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks/exclusiveLocks.feature#L47)
- [coreApiWebdavLocks/exclusiveLocks.feature:48](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks/exclusiveLocks.feature#L48)
- [coreApiWebdavLocks/exclusiveLocks.feature:76](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks/exclusiveLocks.feature#L76)
- [coreApiWebdavLocks/exclusiveLocks.feature:73](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks/exclusiveLocks.feature#L73)
- [coreApiWebdavLocks/exclusiveLocks.feature:74](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks/exclusiveLocks.feature#L74)
- [coreApiWebdavLocks/exclusiveLocks.feature:75](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks/exclusiveLocks.feature#L75)
- [coreApiWebdavLocks/exclusiveLocks.feature:103](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks/exclusiveLocks.feature#L103)
- [coreApiWebdavLocks/exclusiveLocks.feature:100](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks/exclusiveLocks.feature#L100)
- [coreApiWebdavLocks/exclusiveLocks.feature:101](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks/exclusiveLocks.feature#L101)
- [coreApiWebdavLocks/exclusiveLocks.feature:102](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks/exclusiveLocks.feature#L102)
- [coreApiWebdavLocks/requestsWithToken.feature:29](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks/requestsWithToken.feature#L29)
- [coreApiWebdavLocks/requestsWithToken.feature:30](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks/requestsWithToken.feature#L30)
- [coreApiWebdavLocks2/independentLocks.feature:23](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks2/independentLocks.feature#L23)
- [coreApiWebdavLocks2/independentLocks.feature:24](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks2/independentLocks.feature#L24)
- [coreApiWebdavLocks2/independentLocks.feature:25](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks2/independentLocks.feature#L25)
- [coreApiWebdavLocks2/independentLocks.feature:26](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks2/independentLocks.feature#L26)
- [coreApiWebdavLocks2/independentLocks.feature:51](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks2/independentLocks.feature#L51)
- [coreApiWebdavLocks2/independentLocks.feature:52](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks2/independentLocks.feature#L52)
- [coreApiWebdavLocks2/independentLocks.feature:53](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks2/independentLocks.feature#L53)
- [coreApiWebdavLocks2/independentLocks.feature:54](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks2/independentLocks.feature#L54)
- [coreApiWebdavLocks2/independentLocks.feature:55](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks2/independentLocks.feature#L55)
- [coreApiWebdavLocks2/independentLocks.feature:56](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks2/independentLocks.feature#L56)
- [coreApiWebdavLocks2/independentLocks.feature:57](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks2/independentLocks.feature#L57)
- [coreApiWebdavLocks2/independentLocks.feature:58](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks2/independentLocks.feature#L58)
- [coreApiWebdavLocks2/independentLocksShareToShares.feature:30](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks2/independentLocksShareToShares.feature#L30)
- [coreApiWebdavLocks2/independentLocksShareToShares.feature:31](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks2/independentLocksShareToShares.feature#L31)
- [coreApiWebdavLocks2/independentLocksShareToShares.feature:32](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks2/independentLocksShareToShares.feature#L32)
- [coreApiWebdavLocks2/independentLocksShareToShares.feature:33](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks2/independentLocksShareToShares.feature#L33)
- [coreApiWebdavLocks2/independentLocksShareToShares.feature:59](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks2/independentLocksShareToShares.feature#L59)
- [coreApiWebdavLocks2/independentLocksShareToShares.feature:60](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks2/independentLocksShareToShares.feature#L60)
- [coreApiWebdavLocks2/independentLocksShareToShares.feature:61](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks2/independentLocksShareToShares.feature#L61)
- [coreApiWebdavLocks2/independentLocksShareToShares.feature:62](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocks2/independentLocksShareToShares.feature#L62)
- [coreApiWebdavLocksUnlock/unlock.feature:20](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlock.feature#L20)
- [coreApiWebdavLocksUnlock/unlock.feature:21](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlock.feature#L21)
- [coreApiWebdavLocksUnlock/unlock.feature:40](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlock.feature#L40)
- [coreApiWebdavLocksUnlock/unlock.feature:41](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlock.feature#L41)
- [coreApiWebdavLocksUnlock/unlock.feature:63](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlock.feature#L63)
- [coreApiWebdavLocksUnlock/unlock.feature:64](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlock.feature#L64)
- [coreApiWebdavLocksUnlock/unlock.feature:65](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlock.feature#L65)
- [coreApiWebdavLocksUnlock/unlock.feature:66](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlock.feature#L66)
- [coreApiWebdavLocksUnlock/unlockSharingToShares.feature:27](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlockSharingToShares.feature#L27)
- [coreApiWebdavLocksUnlock/unlockSharingToShares.feature:28](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlockSharingToShares.feature#L28)
- [coreApiWebdavLocksUnlock/unlockSharingToShares.feature:29](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlockSharingToShares.feature#L29)
- [coreApiWebdavLocksUnlock/unlockSharingToShares.feature:30](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlockSharingToShares.feature#L30)
- [coreApiWebdavLocksUnlock/unlockSharingToShares.feature:51](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlockSharingToShares.feature#L51)
- [coreApiWebdavLocksUnlock/unlockSharingToShares.feature:52](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlockSharingToShares.feature#L52)
- [coreApiWebdavLocksUnlock/unlockSharingToShares.feature:53](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlockSharingToShares.feature#L53)
- [coreApiWebdavLocksUnlock/unlockSharingToShares.feature:54](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlockSharingToShares.feature#L54)
- [coreApiWebdavLocksUnlock/unlockSharingToShares.feature:75](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlockSharingToShares.feature#L75)
- [coreApiWebdavLocksUnlock/unlockSharingToShares.feature:76](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlockSharingToShares.feature#L76)
- [coreApiWebdavLocksUnlock/unlockSharingToShares.feature:77](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlockSharingToShares.feature#L77)
- [coreApiWebdavLocksUnlock/unlockSharingToShares.feature:78](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlockSharingToShares.feature#L78)
- [coreApiWebdavLocksUnlock/unlockSharingToShares.feature:99](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlockSharingToShares.feature#L99)
- [coreApiWebdavLocksUnlock/unlockSharingToShares.feature:100](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlockSharingToShares.feature#L100)
- [coreApiWebdavLocksUnlock/unlockSharingToShares.feature:101](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlockSharingToShares.feature#L101)
- [coreApiWebdavLocksUnlock/unlockSharingToShares.feature:102](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlockSharingToShares.feature#L102)
- [coreApiWebdavLocksUnlock/unlockSharingToShares.feature:123](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlockSharingToShares.feature#L123)
- [coreApiWebdavLocksUnlock/unlockSharingToShares.feature:124](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlockSharingToShares.feature#L124)
- [coreApiWebdavLocksUnlock/unlockSharingToShares.feature:125](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlockSharingToShares.feature#L125)
- [coreApiWebdavLocksUnlock/unlockSharingToShares.feature:126](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavLocksUnlock/unlockSharingToShares.feature#L126)

### Share
File and sync features in a shared scenario

### [Different response containing exact and non exact match in response of getting sharees](https://github.com/owncloud/ocis/issues/2376)
- [coreApiSharees/sharees.feature:98](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharees/sharees.feature#L98)
- [coreApiSharees/sharees.feature:99](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharees/sharees.feature#L99)
- [coreApiSharees/sharees.feature:118](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharees/sharees.feature#L118)
- [coreApiSharees/sharees.feature:119](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharees/sharees.feature#L119)
- [coreApiSharees/sharees.feature:138](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharees/sharees.feature#L138)
- [coreApiSharees/sharees.feature:139](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharees/sharees.feature#L139)
- [coreApiSharees/sharees.feature:158](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharees/sharees.feature#L158)
- [coreApiSharees/sharees.feature:159](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharees/sharees.feature#L159)
- [coreApiSharees/sharees.feature:178](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharees/sharees.feature#L178)
- [coreApiSharees/sharees.feature:179](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharees/sharees.feature#L179)

#### [accepting matching name shared resources from different users/groups sets no serial identifiers on the resource name for the receiver](https://github.com/owncloud/ocis/issues/4289)
- [coreApiShareManagementToShares/acceptShares.feature:285](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementToShares/acceptShares.feature#L285)
- [coreApiShareManagementToShares/acceptShares.feature:315](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementToShares/acceptShares.feature#L315)

#### [Getting content of a shared file with same name returns 500](https://github.com/owncloud/ocis/issues/3880)
- [coreApiShareManagementToShares/acceptShares.feature:494](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementToShares/acceptShares.feature#L494)
- [coreApiShareManagementToShares/acceptShares.feature:559](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementToShares/acceptShares.feature#L559)
- [coreApiShareCreateSpecialToShares2/createShareReceivedInMultipleWays.feature:129](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareCreateSpecialToShares2/createShareReceivedInMultipleWays.feature#L129)
- [coreApiShareCreateSpecialToShares2/createShareReceivedInMultipleWays.feature:128](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareCreateSpecialToShares2/createShareReceivedInMultipleWays.feature#L128)
- [coreApiShareCreateSpecialToShares2/createShareReceivedInMultipleWays.feature:164](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareCreateSpecialToShares2/createShareReceivedInMultipleWays.feature#L164)
- [coreApiShareCreateSpecialToShares2/createShareReceivedInMultipleWays.feature:163](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareCreateSpecialToShares2/createShareReceivedInMultipleWays.feature#L163)
- [coreApiShareCreateSpecialToShares2/createShareReceivedInMultipleWays.feature:38](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareCreateSpecialToShares2/createShareReceivedInMultipleWays.feature#L38)
- [coreApiShareCreateSpecialToShares2/createShareReceivedInMultipleWays.feature:37](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareCreateSpecialToShares2/createShareReceivedInMultipleWays.feature#L37)

#### [file_target in share response](https://github.com/owncloud/product/issues/203) //todo

- [coreApiShareManagementBasicToShares/createShareToSharesFolder.feature:35](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/createShareToSharesFolder.feature#L35)
- [coreApiShareManagementBasicToShares/createShareToSharesFolder.feature:36](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/createShareToSharesFolder.feature#L36)
- [coreApiShareManagementBasicToShares/deleteShareFromShares.feature:59](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/deleteShareFromShares.feature#L59)

#### [different webdav permissions in ocis](https://github.com/owncloud/ocis/issues/4929)
- [coreApiShareManagementToShares/mergeShare.feature:112](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementToShares/mergeShare.feature#L112)

#### [file_target of a auto-renamed file is not correct directly after sharing](https://github.com/owncloud/ocis/issues/32322)

- [coreApiShareManagementToShares/mergeShare.feature:115](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementToShares/mergeShare.feature#L115)

#### [not possible to move file into a received folder](https://github.com/owncloud/ocis/issues/764)
- [coreApiShareManagementBasicToShares/createShareToSharesFolder.feature:532](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/createShareToSharesFolder.feature#L532)

#### [path property in pending shares gives only filename](https://github.com/owncloud/ocis/issues/2156)
- [coreApiShareManagementBasicToShares/createShareToSharesFolder.feature:742](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/createShareToSharesFolder.feature#L742)
- [coreApiShareManagementBasicToShares/createShareToSharesFolder.feature:741](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/createShareToSharesFolder.feature#L741)
- [coreApiShareManagementBasicToShares/createShareToSharesFolder.feature:761](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/createShareToSharesFolder.feature#L761)
- [coreApiShareManagementBasicToShares/createShareToSharesFolder.feature:760](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/createShareToSharesFolder.feature#L760)
- [coreApiShareManagementBasicToShares/createShareToSharesFolder.feature:777](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/createShareToSharesFolder.feature#L777)
- [coreApiShareManagementBasicToShares/createShareToSharesFolder.feature:776](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/createShareToSharesFolder.feature#L776)

#### [File deletion using dav gives unique string in filename in the trashbin](https://github.com/owncloud/product/issues/178)
- [coreApiShareManagementBasicToShares/deleteShareFromShares.feature:62](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/deleteShareFromShares.feature#L62)
- [coreApiShareManagementBasicToShares/deleteShareFromShares.feature:76](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/deleteShareFromShares.feature#L76)
 cannot share a folder with create permission
#### [Listing shares via ocs API does not show path for parent folders](https://github.com/owncloud/ocis/issues/1231)
- [coreApiShareManagementBasicToShares/deleteShareFromShares.feature:129](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/deleteShareFromShares.feature#L129)
- [coreApiShareManagementBasicToShares/deleteShareFromShares.feature:142](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/deleteShareFromShares.feature#L142)
- [coreApiShareManagementBasicToShares/deleteShareFromShares.feature:177](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/deleteShareFromShares.feature#L177)
- [coreApiShareManagementBasicToShares/deleteShareFromShares.feature:178](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/deleteShareFromShares.feature#L178)
- [coreApiShareManagementBasicToShares/deleteShareFromShares.feature:179](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/deleteShareFromShares.feature#L179)
- [coreApiShareManagementBasicToShares/deleteShareFromShares.feature:176](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/deleteShareFromShares.feature#L176)
- [coreApiShareManagementBasicToShares/deleteShareFromShares.feature:196](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/deleteShareFromShares.feature#L196)
- [coreApiShareManagementBasicToShares/deleteShareFromShares.feature:197](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/deleteShareFromShares.feature#L197)
- [coreApiShareManagementBasicToShares/deleteShareFromShares.feature:198](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/deleteShareFromShares.feature#L198)
- [coreApiShareManagementBasicToShares/deleteShareFromShares.feature:195](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/deleteShareFromShares.feature#L195)
- [coreApiShareOperationsToShares1/gettingShares.feature:188](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares1/gettingShares.feature#L188)
- [coreApiShareOperationsToShares1/gettingShares.feature:187](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares1/gettingShares.feature#L187)
- [coreApiShareManagementBasicToShares/createShareToSharesFolder.feature:325](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/createShareToSharesFolder.feature#L325)
- [coreApiShareManagementBasicToShares/createShareToSharesFolder.feature:326](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/createShareToSharesFolder.feature#L326)

#### [OCS error message for attempting to access share via share id as an unauthorized user is not informative](https://github.com/owncloud/ocis/issues/1233)

- [coreApiShareOperationsToShares1/gettingShares.feature:151](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares1/gettingShares.feature#L151)
- [coreApiShareOperationsToShares1/gettingShares.feature:150](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares1/gettingShares.feature#L150)


#### [Public link enforce permissions](https://github.com/owncloud/ocis/issues/1269)

- [coreApiSharePublicLink1/accessToPublicLinkShare.feature:10](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharePublicLink1/accessToPublicLinkShare.feature#L10)
- [coreApiSharePublicLink1/accessToPublicLinkShare.feature:29](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharePublicLink1/accessToPublicLinkShare.feature#L29)
- [coreApiSharePublicLink1/createPublicLinkShare.feature:167](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharePublicLink1/createPublicLinkShare.feature#L167)
- [coreApiSharePublicLink1/createPublicLinkShare.feature:168](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharePublicLink1/createPublicLinkShare.feature#L168)
- [coreApiSharePublicLink1/createPublicLinkShare.feature:343](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharePublicLink1/createPublicLinkShare.feature#L343)
- [coreApiSharePublicLink1/createPublicLinkShare.feature:353](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharePublicLink1/createPublicLinkShare.feature#L353)

#### [Ability to return error messages in Webdav response bodies](https://github.com/owncloud/ocis/issues/1293)

- [coreApiSharePublicLink1/createPublicLinkShare.feature:68](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharePublicLink1/createPublicLinkShare.feature#L68)
- [coreApiSharePublicLink1/createPublicLinkShare.feature:69](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharePublicLink1/createPublicLinkShare.feature#L69)

#### [copying a folder within a public link folder to folder with same name as an already existing file overwrites the parent file](https://github.com/owncloud/ocis/issues/1232)

- [coreApiSharePublicLink2/copyFromPublicLink.feature:63](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharePublicLink2/copyFromPublicLink.feature#L63)
- [coreApiSharePublicLink2/copyFromPublicLink.feature:89](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharePublicLink2/copyFromPublicLink.feature#L89)
- [coreApiSharePublicLink2/copyFromPublicLink.feature:173](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharePublicLink2/copyFromPublicLink.feature#L173)
- [coreApiSharePublicLink2/copyFromPublicLink.feature:174](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharePublicLink2/copyFromPublicLink.feature#L174)
- [coreApiSharePublicLink2/copyFromPublicLink.feature:189](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharePublicLink2/copyFromPublicLink.feature#L189)
- [coreApiSharePublicLink2/copyFromPublicLink.feature:190](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharePublicLink2/copyFromPublicLink.feature#L190)

#### [Increasing permission of a public link of a folder that was initially shared with share+read permissions is allowed](https://github.com/owncloud/ocis/issues/3881)

- [coreApiSharePublicLink2/reShareAsPublicLinkToSharesNewDav.feature:159](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharePublicLink2/reShareAsPublicLinkToSharesNewDav.feature#L159)
- [coreApiSharePublicLink2/reShareAsPublicLinkToSharesNewDav.feature:160](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharePublicLink2/reShareAsPublicLinkToSharesNewDav.feature#L160)
- [coreApiSharePublicLink2/reShareAsPublicLinkToSharesNewDav.feature:181](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharePublicLink2/reShareAsPublicLinkToSharesNewDav.feature#L181)
- [coreApiSharePublicLink2/reShareAsPublicLinkToSharesNewDav.feature:182](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharePublicLink2/reShareAsPublicLinkToSharesNewDav.feature#L182)

#### [Adding public upload to a read only shared folder as a receipient is allowed ](https://github.com/owncloud/ocis/issues/2164)

- [coreApiSharePublicLink3/updatePublicLinkShare.feature:272](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharePublicLink3/updatePublicLinkShare.feature#L272)
- [coreApiSharePublicLink3/updatePublicLinkShare.feature:271](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharePublicLink3/updatePublicLinkShare.feature#L271)
- [coreApiSharePublicLink3/updatePublicLinkShare.feature:319](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharePublicLink3/updatePublicLinkShare.feature#L319)
- [coreApiSharePublicLink3/updatePublicLinkShare.feature:320](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharePublicLink3/updatePublicLinkShare.feature#L320)

#### [Upload-only shares must not overwrite but create a separate file](https://github.com/owncloud/ocis/issues/1267)

- [coreApiSharePublicLink3/uploadToPublicLinkShare.feature:10](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharePublicLink3/uploadToPublicLinkShare.feature#L10)
- [coreApiSharePublicLink3/uploadToPublicLinkShare.feature:131](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharePublicLink3/uploadToPublicLinkShare.feature#L131)

#### [Set quota over settings](https://github.com/owncloud/ocis/issues/1290)
_requires a [CS3 user provisioning api that can update the quota for a user](https://github.com/cs3org/cs3apis/pull/95#issuecomment-772780683)_

- [coreApiSharePublicLink3/uploadToPublicLinkShare.feature:84](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharePublicLink3/uploadToPublicLinkShare.feature#L84)
- [coreApiSharePublicLink3/uploadToPublicLinkShare.feature:93](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharePublicLink3/uploadToPublicLinkShare.feature#L93)

#### [share permissions are not enforced](https://github.com/owncloud/product/issues/270) //todo

- [coreApiShareReshareToShares3/reShareUpdate.feature:64](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares3/reShareUpdate.feature#L64)
- [coreApiShareReshareToShares3/reShareUpdate.feature:63](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares3/reShareUpdate.feature#L63)

#### [file_target in share response](https://github.com/owncloud/product/issues/203) //todo
- [coreApiShareReshareToShares2/reShareSubfolder.feature:154](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares2/reShareSubfolder.feature#L154)
- [coreApiShareReshareToShares2/reShareSubfolder.feature:153](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares2/reShareSubfolder.feature#L153)

#### [deleting a file inside a received shared folder is moved to the trash-bin of the sharer not the receiver](https://github.com/owncloud/ocis/issues/1124)

- [coreApiTrashbin/trashbinSharingToShares.feature:45](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinSharingToShares.feature#L45)
- [coreApiTrashbin/trashbinSharingToShares.feature:72](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinSharingToShares.feature#L72)
- [coreApiTrashbin/trashbinSharingToShares.feature:99](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinSharingToShares.feature#L99)
- [coreApiTrashbin/trashbinSharingToShares.feature:127](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinSharingToShares.feature#L127)
- [coreApiTrashbin/trashbinSharingToShares.feature:155](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinSharingToShares.feature#L155)
- [coreApiTrashbin/trashbinSharingToShares.feature:183](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinSharingToShares.feature#L183)

#### [Folder overwrite on shared files doesn't works correctly on copying file](https://github.com/owncloud/ocis/issues/2183)
- [coreApiWebdavProperties1/copyFile.feature:466](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/copyFile.feature#L466)
- [coreApiWebdavProperties1/copyFile.feature:548](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/copyFile.feature#L548)

#### [cannot get share-types webdav property](https://github.com/owncloud/ocis/issues/567)
- [coreApiWebdavProperties2/getFileProperties.feature:238](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties2/getFileProperties.feature#L238)
- [coreApiWebdavProperties2/getFileProperties.feature:239](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties2/getFileProperties.feature#L239)

#### [Scoped links](https://github.com/owncloud/ocis/issues/2809)
#### [oc:privatelink property not returned in webdav responses](https://github.com/owncloud/product/issues/262)
- [coreApiWebdavProperties2/getFileProperties.feature:294](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties2/getFileProperties.feature#L294)
- [coreApiWebdavProperties2/getFileProperties.feature:295](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties2/getFileProperties.feature#L295)

#### [changing user quota gives ocs status 103 / Cannot set quota](https://github.com/owncloud/product/issues/247)
_requires a [CS3 user provisioning api that can update the quota for a user](https://github.com/cs3org/cs3apis/pull/95#issuecomment-772780683)_
- [coreApiShareOperationsToShares2/uploadToShare.feature:210](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/uploadToShare.feature#L210)
- [coreApiShareOperationsToShares2/uploadToShare.feature:209](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/uploadToShare.feature#L209)

#### [not possible to move file into a received folder](https://github.com/owncloud/ocis/issues/764)

- [coreApiShareOperationsToShares1/changingFilesShare.feature:25](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares1/changingFilesShare.feature#L25)
- [coreApiShareOperationsToShares1/changingFilesShare.feature:24](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares1/changingFilesShare.feature#L24)
- [coreApiShareOperationsToShares1/changingFilesShare.feature:68](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares1/changingFilesShare.feature#L68)
- [coreApiShareOperationsToShares1/changingFilesShare.feature:69](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares1/changingFilesShare.feature#L69)
- [coreApiShareOperationsToShares1/changingFilesShare.feature:91](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares1/changingFilesShare.feature#L91)
- [coreApiShareOperationsToShares1/changingFilesShare.feature:90](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares1/changingFilesShare.feature#L90)
- [coreApiShareOperationsToShares1/changingFilesShare.feature:107](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares1/changingFilesShare.feature#L107)
- [coreApiShareOperationsToShares1/changingFilesShare.feature:106](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares1/changingFilesShare.feature#L106)
- [coreApiWebdavMove2/moveShareOnOcis.feature:30](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove2/moveShareOnOcis.feature#L30)
- [coreApiWebdavMove2/moveShareOnOcis.feature:32](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove2/moveShareOnOcis.feature#L32)
- [coreApiWebdavMove2/moveShareOnOcis.feature:75](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove2/moveShareOnOcis.feature#L75)
- [coreApiWebdavMove2/moveShareOnOcis.feature:76](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove2/moveShareOnOcis.feature#L76)
- [coreApiWebdavMove2/moveShareOnOcis.feature:98](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove2/moveShareOnOcis.feature#L98)
- [coreApiWebdavMove2/moveShareOnOcis.feature:100](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove2/moveShareOnOcis.feature#L100)
- [coreApiWebdavMove2/moveShareOnOcis.feature:149](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove2/moveShareOnOcis.feature#L149)
- [coreApiWebdavMove2/moveShareOnOcis.feature:150](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove2/moveShareOnOcis.feature#L150)
- [coreApiWebdavMove2/moveShareOnOcis.feature:169](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove2/moveShareOnOcis.feature#L169)
- [coreApiWebdavMove2/moveShareOnOcis.feature:170](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove2/moveShareOnOcis.feature#L170)
- [coreApiWebdavMove2/moveFile.feature:176](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove2/moveFile.feature#L176)
- [coreApiWebdavMove2/moveFile.feature:177](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove2/moveFile.feature#L177)

#### [OCIS-storage overwriting a file as share receiver, does not create a new file version for the sharer](https://github.com/owncloud/ocis/issues/766) //todo
- [coreApiVersions/fileVersions.feature:291](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiVersions/fileVersions.feature#L291)

#### [restoring an older version of a shared file deletes the share](https://github.com/owncloud/ocis/issues/765)
- [coreApiShareManagementToShares/acceptShares.feature:483](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementToShares/acceptShares.feature#L483)
- [coreApiVersions/fileVersions.feature:302](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiVersions/fileVersions.feature#L302)

#### [Expiration date for shares is not implemented](https://github.com/owncloud/ocis/issues/1250)
#### Expiration date of user shares
- [coreApiShareReshareToShares3/reShareWithExpiryDate.feature:34](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares3/reShareWithExpiryDate.feature#L34)
- [coreApiShareReshareToShares3/reShareWithExpiryDate.feature:33](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares3/reShareWithExpiryDate.feature#L33)
- [coreApiShareReshareToShares3/reShareWithExpiryDate.feature:86](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares3/reShareWithExpiryDate.feature#L86)
- [coreApiShareReshareToShares3/reShareWithExpiryDate.feature:87](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares3/reShareWithExpiryDate.feature#L87)
- [coreApiShareReshareToShares3/reShareWithExpiryDate.feature:88](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares3/reShareWithExpiryDate.feature#L88)
- [coreApiShareReshareToShares3/reShareWithExpiryDate.feature:85](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares3/reShareWithExpiryDate.feature#L85)
- [coreApiShareReshareToShares3/reShareWithExpiryDate.feature:143](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares3/reShareWithExpiryDate.feature#L143)
- [coreApiShareReshareToShares3/reShareWithExpiryDate.feature:144](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares3/reShareWithExpiryDate.feature#L144)
- [coreApiShareReshareToShares3/reShareWithExpiryDate.feature:145](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares3/reShareWithExpiryDate.feature#L145)
- [coreApiShareReshareToShares3/reShareWithExpiryDate.feature:142](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares3/reShareWithExpiryDate.feature#L142)
- [coreApiShareReshareToShares3/reShareWithExpiryDate.feature:201](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares3/reShareWithExpiryDate.feature#L201)
- [coreApiShareReshareToShares3/reShareWithExpiryDate.feature:202](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares3/reShareWithExpiryDate.feature#L202)
- [coreApiShareReshareToShares3/reShareWithExpiryDate.feature:203](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares3/reShareWithExpiryDate.feature#L203)
- [coreApiShareReshareToShares3/reShareWithExpiryDate.feature:200](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares3/reShareWithExpiryDate.feature#L200)
- [coreApiShareReshareToShares3/reShareWithExpiryDate.feature:257](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares3/reShareWithExpiryDate.feature#L257)
- [coreApiShareReshareToShares3/reShareWithExpiryDate.feature:258](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares3/reShareWithExpiryDate.feature#L258)
- [coreApiShareReshareToShares3/reShareWithExpiryDate.feature:259](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares3/reShareWithExpiryDate.feature#L259)
- [coreApiShareReshareToShares3/reShareWithExpiryDate.feature:256](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares3/reShareWithExpiryDate.feature#L256)
- [coreApiShareReshareToShares3/reShareWithExpiryDate.feature:287](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares3/reShareWithExpiryDate.feature#L287)
- [coreApiShareReshareToShares3/reShareWithExpiryDate.feature:288](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares3/reShareWithExpiryDate.feature#L288)
- [coreApiShareReshareToShares3/reShareWithExpiryDate.feature:289](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares3/reShareWithExpiryDate.feature#L289)
- [coreApiShareReshareToShares3/reShareWithExpiryDate.feature:286](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares3/reShareWithExpiryDate.feature#L286)
- [coreApiShareReshareToShares3/reShareWithExpiryDate.feature:318](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares3/reShareWithExpiryDate.feature#L318)
- [coreApiShareReshareToShares3/reShareWithExpiryDate.feature:319](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares3/reShareWithExpiryDate.feature#L319)
- [coreApiShareReshareToShares3/reShareWithExpiryDate.feature:320](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares3/reShareWithExpiryDate.feature#L320)
- [coreApiShareReshareToShares3/reShareWithExpiryDate.feature:317](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares3/reShareWithExpiryDate.feature#L317)
- [coreApiShareReshareToShares3/reShareWithExpiryDate.feature:346](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares3/reShareWithExpiryDate.feature#L346)
- [coreApiShareReshareToShares3/reShareWithExpiryDate.feature:347](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares3/reShareWithExpiryDate.feature#L347)
- [coreApiShareReshareToShares3/reShareWithExpiryDate.feature:348](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares3/reShareWithExpiryDate.feature#L348)
- [coreApiShareReshareToShares3/reShareWithExpiryDate.feature:345](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares3/reShareWithExpiryDate.feature#L345)
- [coreApiShareReshareToShares3/reShareWithExpiryDate.feature:379](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares3/reShareWithExpiryDate.feature#L379)
- [coreApiShareReshareToShares3/reShareWithExpiryDate.feature:380](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares3/reShareWithExpiryDate.feature#L380)
- [coreApiShareReshareToShares3/reShareWithExpiryDate.feature:381](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares3/reShareWithExpiryDate.feature#L381)
- [coreApiShareReshareToShares3/reShareWithExpiryDate.feature:382](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares3/reShareWithExpiryDate.feature#L382)
- [coreApiShareReshareToShares3/reShareWithExpiryDate.feature:383](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares3/reShareWithExpiryDate.feature#L383)
- [coreApiShareReshareToShares3/reShareWithExpiryDate.feature:378](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares3/reShareWithExpiryDate.feature#L378)
- [coreApiShareReshareToShares3/reShareWithExpiryDate.feature:413](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares3/reShareWithExpiryDate.feature#L413)
- [coreApiShareReshareToShares3/reShareWithExpiryDate.feature:414](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares3/reShareWithExpiryDate.feature#L414)
- [coreApiShareReshareToShares3/reShareWithExpiryDate.feature:415](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares3/reShareWithExpiryDate.feature#L415)
- [coreApiShareReshareToShares3/reShareWithExpiryDate.feature:412](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares3/reShareWithExpiryDate.feature#L412)
- [coreApiShareReshareToShares3/reShareWithExpiryDate.feature:444](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares3/reShareWithExpiryDate.feature#L444)
- [coreApiShareReshareToShares3/reShareWithExpiryDate.feature:443](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares3/reShareWithExpiryDate.feature#L443)

#### Expiration date of group shares
- [coreApiShareReshareToShares3/reShareWithExpiryDate.feature:60](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares3/reShareWithExpiryDate.feature#L60)
- [coreApiShareReshareToShares3/reShareWithExpiryDate.feature:59](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares3/reShareWithExpiryDate.feature#L59)
- [coreApiShareReshareToShares3/reShareWithExpiryDate.feature:116](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares3/reShareWithExpiryDate.feature#L116)
- [coreApiShareReshareToShares3/reShareWithExpiryDate.feature:117](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares3/reShareWithExpiryDate.feature#L117)
- [coreApiShareReshareToShares3/reShareWithExpiryDate.feature:118](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares3/reShareWithExpiryDate.feature#L118)
- [coreApiShareReshareToShares3/reShareWithExpiryDate.feature:115](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares3/reShareWithExpiryDate.feature#L115)
- [coreApiShareReshareToShares3/reShareWithExpiryDate.feature:172](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares3/reShareWithExpiryDate.feature#L172)
- [coreApiShareReshareToShares3/reShareWithExpiryDate.feature:173](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares3/reShareWithExpiryDate.feature#L173)
- [coreApiShareReshareToShares3/reShareWithExpiryDate.feature:174](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares3/reShareWithExpiryDate.feature#L174)
- [coreApiShareReshareToShares3/reShareWithExpiryDate.feature:171](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares3/reShareWithExpiryDate.feature#L171)
- [coreApiShareReshareToShares3/reShareWithExpiryDate.feature:232](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares3/reShareWithExpiryDate.feature#L232)
- [coreApiShareReshareToShares3/reShareWithExpiryDate.feature:233](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares3/reShareWithExpiryDate.feature#L233)
- [coreApiShareReshareToShares3/reShareWithExpiryDate.feature:234](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares3/reShareWithExpiryDate.feature#L234)
- [coreApiShareReshareToShares3/reShareWithExpiryDate.feature:231](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares3/reShareWithExpiryDate.feature#L231)


#### [Getting content of a shared file with same name returns 500](https://github.com/owncloud/ocis/issues/3880)
- [coreApiShareCreateSpecialToShares1/createShareUniqueReceivedNames.feature:14](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareCreateSpecialToShares1/createShareUniqueReceivedNames.feature#L14)

#### [Empty OCS response for a share create request using a disabled user](https://github.com/owncloud/ocis/issues/2212)
- [coreApiShareCreateSpecialToShares2/createShareWithDisabledUser.feature:19](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareCreateSpecialToShares2/createShareWithDisabledUser.feature#L19)
- [coreApiShareCreateSpecialToShares2/createShareWithDisabledUser.feature:22](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareCreateSpecialToShares2/createShareWithDisabledUser.feature#L22)

//todo
- [coreApiShareUpdateToShares/updateShare.feature:97](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareUpdateToShares/updateShare.feature#L97)
- [coreApiShareUpdateToShares/updateShare.feature:98](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareUpdateToShares/updateShare.feature#L98)
- [coreApiShareUpdateToShares/updateShare.feature:99](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareUpdateToShares/updateShare.feature#L99)
- [coreApiShareUpdateToShares/updateShare.feature:100](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareUpdateToShares/updateShare.feature#L100)
- [coreApiShareUpdateToShares/updateShare.feature:95](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareUpdateToShares/updateShare.feature#L95)
- [coreApiShareUpdateToShares/updateShare.feature:96](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareUpdateToShares/updateShare.feature#L96)
- [coreApiShareUpdateToShares/updateShare.feature:121](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareUpdateToShares/updateShare.feature#L121)
- [coreApiShareUpdateToShares/updateShare.feature:122](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareUpdateToShares/updateShare.feature#L122)
- [coreApiShareUpdateToShares/updateShare.feature:123](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareUpdateToShares/updateShare.feature#L123)
- [coreApiShareUpdateToShares/updateShare.feature:124](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareUpdateToShares/updateShare.feature#L124)
- [coreApiShareUpdateToShares/updateShare.feature:119](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareUpdateToShares/updateShare.feature#L119)
- [coreApiShareUpdateToShares/updateShare.feature:120](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareUpdateToShares/updateShare.feature#L120)

#### [Edit user share response has an "name" field](https://github.com/owncloud/ocis/issues/1225)
- [coreApiShareUpdateToShares/updateShare.feature:242](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareUpdateToShares/updateShare.feature#L242)
- [coreApiShareUpdateToShares/updateShare.feature:241](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareUpdateToShares/updateShare.feature#L241)

#### [user can access version metadata of a received share before accepting it](https://github.com/owncloud/ocis/issues/760)
- [coreApiVersions/fileVersions.feature:487](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiVersions/fileVersions.feature#L487)

#### [Share lists deleted user as 'user'](https://github.com/owncloud/ocis/issues/903)
- [coreApiShareManagementBasicToShares/createShareToSharesFolder.feature:677](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/createShareToSharesFolder.feature#L677)
- [coreApiShareManagementBasicToShares/createShareToSharesFolder.feature:676](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/createShareToSharesFolder.feature#L676)

#### [OCIS-storage overwriting a file as share receiver, does not create a new file version for the sharer](https://github.com/owncloud/ocis/issues/766) //todo
- [coreApiVersions/fileVersions.feature:499](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiVersions/fileVersions.feature#L499)

#### [deleting a share with wrong authentication returns OCS status 996 / HTTP 500](https://github.com/owncloud/ocis/issues/1229)
- [coreApiShareManagementBasicToShares/deleteShareFromShares.feature:226](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/deleteShareFromShares.feature#L226)
- [coreApiShareManagementBasicToShares/deleteShareFromShares.feature:225](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/deleteShareFromShares.feature#L225)

### User Management
User and group management features

### Other
API, search, favorites, config, capabilities, not existing endpoints, CORS and others

#### [no robots.txt available](https://github.com/owncloud/ocis/issues/1314)
- [coreApiMain/main.feature:5](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiMain/main.feature#L5) Scenario: robots.txt file should be accessible

#### [Ability to return error messages in Webdav response bodies](https://github.com/owncloud/ocis/issues/1293)
- [coreApiAuthOcs/ocsDELETEAuth.feature:8](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthOcs/ocsDELETEAuth.feature#L8) Scenario: send DELETE requests to OCS endpoints as admin with wrong password
- [coreApiAuthOcs/ocsGETAuth.feature:8](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthOcs/ocsGETAuth.feature#L8) Scenario: using OCS anonymously
- [coreApiAuthOcs/ocsGETAuth.feature:29](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthOcs/ocsGETAuth.feature#L29) Scenario: ocs config end point accessible by unauthorized users
- [coreApiAuthOcs/ocsGETAuth.feature:42](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthOcs/ocsGETAuth.feature#L42) Scenario: using OCS with non-admin basic auth
- [coreApiAuthOcs/ocsGETAuth.feature:73](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthOcs/ocsGETAuth.feature#L73) Scenario: using OCS as normal user with wrong password
- [coreApiAuthOcs/ocsGETAuth.feature:104](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthOcs/ocsGETAuth.feature#L104) Scenario:using OCS with admin basic auth
- [coreApiAuthOcs/ocsGETAuth.feature:121](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthOcs/ocsGETAuth.feature#L121) Scenario: using OCS as admin user with wrong password
- [coreApiAuthOcs/ocsPOSTAuth.feature:8](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthOcs/ocsPOSTAuth.feature#L8) Scenario: send POST requests to OCS endpoints as normal user with wrong password
- [coreApiAuthOcs/ocsPUTAuth.feature:8](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthOcs/ocsPUTAuth.feature#L8) Scenario: send PUT request to OCS endpoints as admin with wrong password

#### [sending MKCOL requests to another or non-existing user's webDav endpoints as normal user should return 404](https://github.com/owncloud/ocis/issues/5049)
_ocdav: api compatibility, return correct status code_
- [coreApiAuthWebDav/webDavDELETEAuth.feature:57](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthWebDav/webDavDELETEAuth.feature#L57) Scenario: send DELETE requests to another user's webDav endpoints as normal user
- [coreApiAuthWebDav/webDavPROPFINDAuth.feature:55](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthWebDav/webDavPROPFINDAuth.feature#L55)  Scenario: send PROPFIND requests to another user's webDav endpoints as normal user
- [coreApiAuthWebDav/webDavPROPPATCHAuth.feature:56](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthWebDav/webDavPROPPATCHAuth.feature#L56) Scenario: send PROPPATCH requests to another user's webDav endpoints as normal user
- [coreApiAuthWebDav/webDavMKCOLAuth.feature:52](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthWebDav/webDavMKCOLAuth.feature#L52) Scenario: send MKCOL requests to another user's webDav endpoints as normal user
- [coreApiAuthWebDav/webDavMKCOLAuth.feature:63](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthWebDav/webDavMKCOLAuth.feature#L63) Scenario: send MKCOL requests to another user's webDav endpoints as normal user using the spaces WebDAV API

#### [trying to lock file of another user gives http 200](https://github.com/owncloud/ocis/issues/2176)
- [coreApiAuthWebDav/webDavLOCKAuth.feature:56](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthWebDav/webDavLOCKAuth.feature#L56) Scenario: send LOCK requests to another user's webDav endpoints as normal user

#### [send (MOVE, COPY) requests to another user's webDav endpoints as normal user gives 400 instead of 403](https://github.com/owncloud/ocis/issues/3882)
_ocdav: api compatibility, return correct status code_
- [coreApiAuthWebDav/webDavMOVEAuth.feature:55](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthWebDav/webDavMOVEAuth.feature#L55) Scenario: send MOVE requests to another user's webDav endpoints as normal user
- [coreApiAuthWebDav/webDavCOPYAuth.feature:59](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthWebDav/webDavCOPYAuth.feature#L59)

#### [send POST requests to another user's webDav endpoints as normal user](https://github.com/owncloud/ocis/issues/1287)
_ocdav: api compatibility, return correct status code_
- [coreApiAuthWebDav/webDavPOSTAuth.feature:56](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthWebDav/webDavPOSTAuth.feature#L56)  Scenario: send POST requests to another user's webDav endpoints as normal user

#### [Using double slash in URL to access a folder gives 501 and other status codes](https://github.com/owncloud/ocis/issues/1667)
- [coreApiAuthWebDav/webDavSpecialURLs.feature:34](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthWebDav/webDavSpecialURLs.feature#L34)
- [coreApiAuthWebDav/webDavSpecialURLs.feature:121](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthWebDav/webDavSpecialURLs.feature#L121)
- [coreApiAuthWebDav/webDavSpecialURLs.feature:163](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthWebDav/webDavSpecialURLs.feature#L163)

#### [Difference in response content of status.php and default capabilities](https://github.com/owncloud/ocis/issues/1286)
- [coreApiCapabilities/capabilitiesWithNormalUser.feature:11](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiCapabilities/capabilitiesWithNormalUser.feature#L11) Scenario: getting default capabilities with normal user

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
- [coreApiFavorites/favorites.feature:167](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favorites.feature#L167)
- [coreApiFavorites/favorites.feature:168](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favorites.feature#L168)
- [coreApiFavorites/favorites.feature:200](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favorites.feature#L200)
- [coreApiFavorites/favorites.feature:201](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favorites.feature#L201)
- [coreApiFavorites/favoritesSharingToShares.feature:82](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favoritesSharingToShares.feature#L82)
- [coreApiFavorites/favoritesSharingToShares.feature:81](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favoritesSharingToShares.feature#L81)

#### [resource inside Shares dir is not found using the spaces WebDAV API](https://github.com/owncloud/ocis/issues/2968)
- [coreApiFavorites/favoritesSharingToShares.feature:22](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favoritesSharingToShares.feature#L22)
- [coreApiFavorites/favoritesSharingToShares.feature:21](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favoritesSharingToShares.feature#L21)
- [coreApiFavorites/favoritesSharingToShares.feature:37](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favoritesSharingToShares.feature#L37)
- [coreApiFavorites/favoritesSharingToShares.feature:36](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favoritesSharingToShares.feature#L36)
- [coreApiFavorites/favoritesSharingToShares.feature:51](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favoritesSharingToShares.feature#L51)
- [coreApiFavorites/favoritesSharingToShares.feature:50](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favoritesSharingToShares.feature#L50)
- [coreApiFavorites/favoritesSharingToShares.feature:67](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favoritesSharingToShares.feature#L67)
- [coreApiFavorites/favoritesSharingToShares.feature:66](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiFavorites/favoritesSharingToShares.feature#L66)
- [coreApiMain/checksums.feature:203](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiMain/checksums.feature#L203)

#### [WWW-Authenticate header for unauthenticated requests is not clear](https://github.com/owncloud/ocis/issues/2285)
- [coreApiWebdavOperations/refuseAccess.feature:22](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/refuseAccess.feature#L22)
- [coreApiWebdavOperations/refuseAccess.feature:23](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/refuseAccess.feature#L23)
- [coreApiWebdavOperations/refuseAccess.feature:35](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/refuseAccess.feature#L35)
- [coreApiWebdavOperations/refuseAccess.feature:36](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/refuseAccess.feature#L36)

#### [App Passwords/Tokens for legacy WebDAV clients](https://github.com/owncloud/ocis/issues/197)
- [coreApiAuthWebDav/webDavDELETEAuth.feature:135](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthWebDav/webDavDELETEAuth.feature#L135)
- [coreApiAuthWebDav/webDavDELETEAuth.feature:161](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthWebDav/webDavDELETEAuth.feature#L161)

#### [Sharing a same file twice to the same group](https://github.com/owncloud/ocis/issues/1710)
- [coreApiShareManagementBasicToShares/createShareToSharesFolder.feature:725](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/createShareToSharesFolder.feature#L725)
- [coreApiShareManagementBasicToShares/createShareToSharesFolder.feature:724](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/createShareToSharesFolder.feature#L724)

#### [Request to edit non-existing user by authorized admin gets unauthorized in http response](https://github.com/owncloud/ocis/issues/38423)
- [coreApiAuthOcs/ocsPUTAuth.feature:24](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthOcs/ocsPUTAuth.feature#L24)


### Won't fix
Not everything needs to be implemented for ocis. While the oc10 testsuite covers these things we are not looking at them right now.

* _The `OC-LazyOps` header is [no longer supported by the client](https://github.com/owncloud/client/pull/8398), implmenting this is not necessary for a first production release. We plan to have an uploed state machine to visualize the state of a file, see https://github.com/owncloud/ocis/issues/214_
* _Blacklisted ignored files are no longer required because ocis can handle `.htaccess` files without security implications introduced by serving user provided files with apache._

#### [Blacklist files extensions](https://github.com/owncloud/ocis/issues/2177)
- [coreApiWebdavProperties1/copyFile.feature:120](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/copyFile.feature#L120)
- [coreApiWebdavProperties1/copyFile.feature:119](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/copyFile.feature#L119)
- [coreApiWebdavProperties1/createFileFolder.feature:98](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/createFileFolder.feature#L98)
- [coreApiWebdavProperties1/createFileFolder.feature:99](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/createFileFolder.feature#L99)
- [coreApiWebdavUpload1/uploadFile.feature:181](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUpload1/uploadFile.feature#L181)
- [coreApiWebdavUpload1/uploadFile.feature:182](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUpload1/uploadFile.feature#L182)

#### [cannot set blacklisted file names](https://github.com/owncloud/product/issues/260)
- [coreApiWebdavMove1/moveFolderToBlacklistedName.feature:21](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove1/moveFolderToBlacklistedName.feature#L21)
- [coreApiWebdavMove1/moveFolderToBlacklistedName.feature:22](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove1/moveFolderToBlacklistedName.feature#L22)

#### [cannot set blacklisted file names](https://github.com/owncloud/product/issues/260)
- [coreApiWebdavMove2/moveFileToBlacklistedName.feature:19](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove2/moveFileToBlacklistedName.feature#L19)
- [coreApiWebdavMove2/moveFileToBlacklistedName.feature:20](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavMove2/moveFileToBlacklistedName.feature#L20)

#### [PATCH request for TUS upload with wrong checksum gives incorrect response](https://github.com/owncloud/ocis/issues/1755)
- [coreApiWebdavUploadTUS/checksums.feature:84](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L84)
- [coreApiWebdavUploadTUS/checksums.feature:85](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L85)
- [coreApiWebdavUploadTUS/checksums.feature:86](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L86)
- [coreApiWebdavUploadTUS/checksums.feature:87](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L87)
- [coreApiWebdavUploadTUS/checksums.feature:173](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L173)
- [coreApiWebdavUploadTUS/checksums.feature:174](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L174)
- [coreApiWebdavUploadTUS/checksums.feature:226](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L226)
- [coreApiWebdavUploadTUS/checksums.feature:227](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L227)
- [coreApiWebdavUploadTUS/checksums.feature:228](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L228)
- [coreApiWebdavUploadTUS/checksums.feature:229](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L229)
- [coreApiWebdavUploadTUS/checksums.feature:282](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L282)
- [coreApiWebdavUploadTUS/checksums.feature:283](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L283)
- [coreApiWebdavUploadTUS/checksums.feature:284](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L284)
- [coreApiWebdavUploadTUS/checksums.feature:285](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/checksums.feature#L285)
- [coreApiWebdavUploadTUS/optionsRequest.feature:8](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/optionsRequest.feature#L8)
- [coreApiWebdavUploadTUS/optionsRequest.feature:35](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/optionsRequest.feature#L35)
- [coreApiWebdavUploadTUS/optionsRequest.feature:62](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/optionsRequest.feature#L62)
- [coreApiWebdavUploadTUS/optionsRequest.feature:89](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/optionsRequest.feature#L89)
- [coreApiWebdavUploadTUS/uploadToShare.feature:176](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/uploadToShare.feature#L176)
- [coreApiWebdavUploadTUS/uploadToShare.feature:175](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/uploadToShare.feature#L175)
- [coreApiWebdavUploadTUS/uploadToShare.feature:195](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/uploadToShare.feature#L195)
- [coreApiWebdavUploadTUS/uploadToShare.feature:194](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/uploadToShare.feature#L194)
- [coreApiWebdavUploadTUS/uploadToShare.feature:214](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/uploadToShare.feature#L214)
- [coreApiWebdavUploadTUS/uploadToShare.feature:213](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/uploadToShare.feature#L213)
- [coreApiWebdavUploadTUS/uploadToShare.feature:253](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/uploadToShare.feature#L253)
- [coreApiWebdavUploadTUS/uploadToShare.feature:252](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/uploadToShare.feature#L252)
- [coreApiWebdavUploadTUS/uploadToShare.feature:295](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/uploadToShare.feature#L295)
- [coreApiWebdavUploadTUS/uploadToShare.feature:294](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavUploadTUS/uploadToShare.feature#L294)

#### [Share inaccessible if folder with same name was deleted and recreated](https://github.com/owncloud/ocis/issues/1787)
- [coreApiShareReshareToShares1/reShare.feature:266](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares1/reShare.feature#L266)
- [coreApiShareReshareToShares1/reShare.feature:265](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares1/reShare.feature#L265)
- [coreApiShareReshareToShares1/reShare.feature:284](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares1/reShare.feature#L284)
- [coreApiShareReshareToShares1/reShare.feature:283](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares1/reShare.feature#L283)
- [coreApiShareReshareToShares1/reShare.feature:302](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares1/reShare.feature#L302)
- [coreApiShareReshareToShares1/reShare.feature:301](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares1/reShare.feature#L301)

#### [incorrect ocs(v2) status value when getting info of share that does not exist should be 404, gives 998](https://github.com/owncloud/product/issues/250)
_ocs: api compatibility, return correct status code_
- [coreApiShareOperationsToShares2/shareAccessByID.feature:48](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L48)
- [coreApiShareOperationsToShares2/shareAccessByID.feature:49](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L49)
- [coreApiShareOperationsToShares2/shareAccessByID.feature:50](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L50)
- [coreApiShareOperationsToShares2/shareAccessByID.feature:51](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L51)
- [coreApiShareOperationsToShares2/shareAccessByID.feature:52](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L52)
- [coreApiShareOperationsToShares2/shareAccessByID.feature:53](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L53)
- [coreApiShareOperationsToShares2/shareAccessByID.feature:54](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L54)
- [coreApiShareOperationsToShares2/shareAccessByID.feature:47](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L47)

#### [Trying to accept a share with invalid ID gives incorrect OCS and HTTP status](https://github.com/owncloud/ocis/issues/2111)
- [coreApiShareOperationsToShares2/shareAccessByID.feature:85](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L85)
- [coreApiShareOperationsToShares2/shareAccessByID.feature:86](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L86)
- [coreApiShareOperationsToShares2/shareAccessByID.feature:87](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L87)
- [coreApiShareOperationsToShares2/shareAccessByID.feature:88](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L88)
- [coreApiShareOperationsToShares2/shareAccessByID.feature:89](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L89)
- [coreApiShareOperationsToShares2/shareAccessByID.feature:90](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L90)
- [coreApiShareOperationsToShares2/shareAccessByID.feature:91](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L91)
- [coreApiShareOperationsToShares2/shareAccessByID.feature:84](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L84)
- [coreApiShareOperationsToShares2/shareAccessByID.feature:104](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L104)
- [coreApiShareOperationsToShares2/shareAccessByID.feature:103](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L103)
- [coreApiShareOperationsToShares2/shareAccessByID.feature:136](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L136)
- [coreApiShareOperationsToShares2/shareAccessByID.feature:137](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L137)
- [coreApiShareOperationsToShares2/shareAccessByID.feature:138](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L138)
- [coreApiShareOperationsToShares2/shareAccessByID.feature:139](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L139)
- [coreApiShareOperationsToShares2/shareAccessByID.feature:140](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L140)
- [coreApiShareOperationsToShares2/shareAccessByID.feature:141](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L141)
- [coreApiShareOperationsToShares2/shareAccessByID.feature:142](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L142)
- [coreApiShareOperationsToShares2/shareAccessByID.feature:135](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L135)
- [coreApiShareOperationsToShares2/shareAccessByID.feature:155](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L155)
- [coreApiShareOperationsToShares2/shareAccessByID.feature:154](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L154)

#### [[OC-storage] share-types field empty for shared file folder in webdav response](https://github.com/owncloud/ocis/issues/2144)
- [coreApiWebdavProperties2/getFileProperties.feature:215](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties2/getFileProperties.feature#L215)
- [coreApiWebdavProperties2/getFileProperties.feature:216](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties2/getFileProperties.feature#L216)

#### [Different share permissions provides varying roles in oc10 and ocis](https://github.com/owncloud/ocis/issues/1277)
- [coreApiWebdavProperties2/getFileProperties.feature:275](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties2/getFileProperties.feature#L275)
- [coreApiWebdavProperties2/getFileProperties.feature:276](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties2/getFileProperties.feature#L276)

#### [Cannot move folder/file from one received share to another](https://github.com/owncloud/ocis/issues/2442)
- [coreApiShareUpdateToShares/updateShare.feature:195](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareUpdateToShares/updateShare.feature#L195)
- [coreApiShareUpdateToShares/updateShare.feature:159](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareUpdateToShares/updateShare.feature#L159)
- [coreApiShareManagementToShares/mergeShare.feature:135](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementToShares/mergeShare.feature#L135)

#### [Sharing folder and sub-folder with same user but different permission,the permission of sub-folder is not obeyed ](https://github.com/owncloud/ocis/issues/2440)
- [coreApiShareCreateSpecialToShares2/createShareReceivedInMultipleWays.feature:260](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareCreateSpecialToShares2/createShareReceivedInMultipleWays.feature#L260)
- [coreApiShareCreateSpecialToShares2/createShareReceivedInMultipleWays.feature:295](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareCreateSpecialToShares2/createShareReceivedInMultipleWays.feature#L295)
- [coreApiShareCreateSpecialToShares2/createShareReceivedInMultipleWays.feature:404](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareCreateSpecialToShares2/createShareReceivedInMultipleWays.feature#L404)
- [coreApiShareCreateSpecialToShares2/createShareReceivedInMultipleWays.feature:439](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareCreateSpecialToShares2/createShareReceivedInMultipleWays.feature#L439)

#### [copying the file inside Shares folder returns 404](https://github.com/owncloud/ocis/issues/3874)
- [coreApiWebdavProperties1/copyFile.feature:410](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/copyFile.feature#L410)
- [coreApiWebdavProperties1/copyFile.feature:409](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/copyFile.feature#L409)
- [coreApiWebdavProperties1/copyFile.feature:436](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/copyFile.feature#L436)
- [coreApiWebdavProperties1/copyFile.feature:435](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/copyFile.feature#L435)

#### [Shares to deleted group listed in the response](https://github.com/owncloud/ocis/issues/2441)
- [coreApiShareManagementBasicToShares/createShareToSharesFolder.feature:529](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/createShareToSharesFolder.feature#L529)
- [coreApiShareManagementBasicToShares/createShareToSharesFolder.feature:528](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementBasicToShares/createShareToSharesFolder.feature#L528)

### [Share path in the response is different between share states](https://github.com/owncloud/ocis/issues/2540)
- [coreApiShareManagementToShares/acceptShares.feature:64](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementToShares/acceptShares.feature#L64)
- [coreApiShareManagementToShares/acceptShares.feature:87](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementToShares/acceptShares.feature#L87)
- [coreApiShareManagementToShares/acceptShares.feature:165](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementToShares/acceptShares.feature#L165)
- [coreApiShareManagementToShares/acceptShares.feature:188](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementToShares/acceptShares.feature#L188)
- [coreApiShareManagementToShares/acceptShares.feature:226](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementToShares/acceptShares.feature#L226)
- [coreApiShareManagementToShares/acceptShares.feature:260](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementToShares/acceptShares.feature#L260)
- [coreApiShareManagementToShares/acceptShares.feature:480](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementToShares/acceptShares.feature#L480)
- [coreApiShareOperationsToShares2/shareAccessByID.feature:123](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L123)
- [coreApiShareOperationsToShares2/shareAccessByID.feature:122](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares2/shareAccessByID.feature#L122)
- [coreApiShareManagementToShares/acceptShares.feature:229](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementToShares/acceptShares.feature#L229)

### [Content-type is not multipart/byteranges when downloading file with Range Header](https://github.com/owncloud/ocis/issues/2677)
- [coreApiWebdavOperations/downloadFile.feature:184](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/downloadFile.feature#L184)
- [coreApiWebdavOperations/downloadFile.feature:185](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/downloadFile.feature#L185)

#### [Creating a new folder which is a substring of Shares leads to Unknown Error](https://github.com/owncloud/ocis/issues/3033)
- [coreApiWebdavProperties1/createFileFolderWhenSharesExist.feature:28](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/createFileFolderWhenSharesExist.feature#L28)
- [coreApiWebdavProperties1/createFileFolderWhenSharesExist.feature:31](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/createFileFolderWhenSharesExist.feature#L31)
- [coreApiWebdavProperties1/createFileFolderWhenSharesExist.feature:44](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/createFileFolderWhenSharesExist.feature#L44)
- [coreApiWebdavProperties1/createFileFolderWhenSharesExist.feature:47](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties1/createFileFolderWhenSharesExist.feature#L47)

#### [moveShareInsideAnotherShare behaves differently on oCIS than oC10](https://github.com/owncloud/ocis/issues/3047)
- [coreApiShareManagementToShares/moveShareInsideAnotherShare.feature:24](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementToShares/moveShareInsideAnotherShare.feature#L24)
- [coreApiShareManagementToShares/moveShareInsideAnotherShare.feature:44](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementToShares/moveShareInsideAnotherShare.feature#L44)
- [coreApiShareManagementToShares/moveShareInsideAnotherShare.feature:58](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementToShares/moveShareInsideAnotherShare.feature#L58)

#### [resource path is included in the returned error message](https://github.com/owncloud/ocis/issues/3344)
- [coreApiWebdavProperties2/getFileProperties.feature:311](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavProperties2/getFileProperties.feature#L311)

#### [OCS status code zero](https://github.com/owncloud/ocis/issues/3621)
- [coreApiShareManagementToShares/moveReceivedShare.feature:14](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareManagementToShares/moveReceivedShare.feature#L14)

#### [HTTP status code differ while listing the contents of another user's trash bin](https://github.com/owncloud/ocis/issues/3561)
- [coreApiTrashbin/trashbinFilesFolders.feature:263](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinFilesFolders.feature#L263)

#### [Cannot disable the dav propfind depth infinity for resources](https://github.com/owncloud/ocis/issues/3720)
- [coreApiWebdavOperations/listFiles.feature:364](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/listFiles.feature#L364)
- [coreApiWebdavOperations/listFiles.feature:365](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/listFiles.feature#L365)
- [coreApiWebdavOperations/listFiles.feature:384](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/listFiles.feature#L384)
- [coreApiWebdavOperations/listFiles.feature:403](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/listFiles.feature#L403)
- [coreApiWebdavOperations/listFiles.feature:404](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/listFiles.feature#L404)

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
- [coreApiTrashbin/trashbinFilesFolders.feature:320](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinFilesFolders.feature#L320)
- [coreApiTrashbin/trashbinFilesFolders.feature:321](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinFilesFolders.feature#L321)
- [coreApiTrashbin/trashbinFilesFolders.feature:322](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinFilesFolders.feature#L322)
- [coreApiTrashbin/trashbinFilesFolders.feature:323](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinFilesFolders.feature#L323)
- [coreApiTrashbin/trashbinFilesFolders.feature:324](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinFilesFolders.feature#L324)
- [coreApiTrashbin/trashbinFilesFolders.feature:319](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinFilesFolders.feature#L319)
- [coreApiTrashbin/trashbinFilesFolders.feature:346](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinFilesFolders.feature#L346)
- [coreApiTrashbin/trashbinFilesFolders.feature:366](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinFilesFolders.feature#L366)
- [coreApiTrashbin/trashbinFilesFolders.feature:420](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinFilesFolders.feature#L420)
- [coreApiTrashbin/trashbinFilesFolders.feature:457](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinFilesFolders.feature#L457)
- [coreApiTrashbin/trashbinSharingToShares.feature:23](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinSharingToShares.feature#L23)
- [coreApiTrashbin/trashbinSharingToShares.feature:206](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinSharingToShares.feature#L206)
- [coreApiTrashbin/trashbinSharingToShares.feature:230](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbin/trashbinSharingToShares.feature#L230)
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
- [coreApiTrashbinRestore/trashbinRestore.feature:180](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L180)
- [coreApiTrashbinRestore/trashbinRestore.feature:181](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L181)
- [coreApiTrashbinRestore/trashbinRestore.feature:195](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L195)
- [coreApiTrashbinRestore/trashbinRestore.feature:196](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L196)
- [coreApiTrashbinRestore/trashbinRestore.feature:210](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L210)
- [coreApiTrashbinRestore/trashbinRestore.feature:211](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L211)
- [coreApiTrashbinRestore/trashbinRestore.feature:227](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L227)
- [coreApiTrashbinRestore/trashbinRestore.feature:228](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L228)
- [coreApiTrashbinRestore/trashbinRestore.feature:229](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L229)
- [coreApiTrashbinRestore/trashbinRestore.feature:230](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L230)
- [coreApiTrashbinRestore/trashbinRestore.feature:225](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L225)
- [coreApiTrashbinRestore/trashbinRestore.feature:226](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L226)
- [coreApiTrashbinRestore/trashbinRestore.feature:247](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L247)
- [coreApiTrashbinRestore/trashbinRestore.feature:248](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L248)
- [coreApiTrashbinRestore/trashbinRestore.feature:265](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L265)
- [coreApiTrashbinRestore/trashbinRestore.feature:266](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L266)
- [coreApiTrashbinRestore/trashbinRestore.feature:285](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L285)
- [coreApiTrashbinRestore/trashbinRestore.feature:286](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L286)
- [coreApiTrashbinRestore/trashbinRestore.feature:305](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L305)
- [coreApiTrashbinRestore/trashbinRestore.feature:306](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L306)
- [coreApiTrashbinRestore/trashbinRestore.feature:339](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L339)
- [coreApiTrashbinRestore/trashbinRestore.feature:340](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L340)
- [coreApiTrashbinRestore/trashbinRestore.feature:378](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L378)
- [coreApiTrashbinRestore/trashbinRestore.feature:379](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L379)
- [coreApiTrashbinRestore/trashbinRestore.feature:417](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L417)
- [coreApiTrashbinRestore/trashbinRestore.feature:418](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L418)
- [coreApiTrashbinRestore/trashbinRestore.feature:435](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L435)
- [coreApiTrashbinRestore/trashbinRestore.feature:436](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L436)
- [coreApiTrashbinRestore/trashbinRestore.feature:454](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L454)
- [coreApiTrashbinRestore/trashbinRestore.feature:455](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L455)
- [coreApiTrashbinRestore/trashbinRestore.feature:478](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L478)
- [coreApiTrashbinRestore/trashbinRestore.feature:479](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L479)
- [coreApiTrashbinRestore/trashbinRestore.feature:497](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L497)
- [coreApiTrashbinRestore/trashbinRestore.feature:498](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L498)
- [coreApiTrashbinRestore/trashbinRestore.feature:512](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L512)
- [coreApiTrashbinRestore/trashbinRestore.feature:513](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L513)
- [coreApiTrashbinRestore/trashbinRestore.feature:566](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L566)
- [coreApiTrashbinRestore/trashbinRestore.feature:567](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiTrashbinRestore/trashbinRestore.feature#L567)
- [coreApiWebdavEtagPropagation2/restoreFromTrash.feature:26](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavEtagPropagation2/restoreFromTrash.feature#L26)
- [coreApiWebdavEtagPropagation2/restoreFromTrash.feature:27](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavEtagPropagation2/restoreFromTrash.feature#L27)
- [coreApiWebdavEtagPropagation2/restoreFromTrash.feature:49](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavEtagPropagation2/restoreFromTrash.feature#L49)
- [coreApiWebdavEtagPropagation2/restoreFromTrash.feature:50](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavEtagPropagation2/restoreFromTrash.feature#L50)
- [coreApiWebdavEtagPropagation2/restoreFromTrash.feature:70](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavEtagPropagation2/restoreFromTrash.feature#L70)
- [coreApiWebdavEtagPropagation2/restoreFromTrash.feature:71](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavEtagPropagation2/restoreFromTrash.feature#L71)
- [coreApiWebdavEtagPropagation2/restoreFromTrash.feature:93](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavEtagPropagation2/restoreFromTrash.feature#L93)
- [coreApiWebdavEtagPropagation2/restoreFromTrash.feature:94](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavEtagPropagation2/restoreFromTrash.feature#L94)
- [coreApiVersions/fileVersions.feature:237](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiVersions/fileVersions.feature#L237)

#### [`meta` requests have empty responses with master branch](https://github.com/cs3org/reva/issues/2897)
- [coreApiVersions/fileVersions.feature:197](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiVersions/fileVersions.feature#L197)
- [coreApiVersions/fileVersions.feature:203](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiVersions/fileVersions.feature#L203)
- [coreApiVersions/fileVersions.feature:210](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiVersions/fileVersions.feature#L210)
- [coreApiVersions/fileVersions.feature:218](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiVersions/fileVersions.feature#L218)
- [coreApiVersions/fileVersions.feature:231](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiVersions/fileVersions.feature#L231)
- [coreApiVersions/fileVersions.feature:232](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiVersions/fileVersions.feature#L232)
- [coreApiVersions/fileVersions.feature:233](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiVersions/fileVersions.feature#L233)
- [coreApiVersions/fileVersions.feature:234](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiVersions/fileVersions.feature#L234)

#### [WebDAV MOVE with body returns 400 rather than 415](https://github.com/cs3org/reva/issues/3119)

- [coreApiAuthWebDav/webDavMOVEAuth.feature:133](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthWebDav/webDavMOVEAuth.feature#L133)

#### [reShareUpdate API tests failing in reva](https://github.com/cs3org/reva/issues/2916)

- [coreApiShareReshareToShares3/reShareUpdate.feature:159](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares3/reShareUpdate.feature#L159)
- [coreApiShareReshareToShares3/reShareUpdate.feature:158](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareReshareToShares3/reShareUpdate.feature#L158)

#### [coreApiShareOperationsToShares1/gettingShares.feature:28 fails in CI](https://github.com/cs3org/reva/issues/2926)

- [coreApiShareOperationsToShares1/gettingShares.feature:38](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares1/gettingShares.feature#L38)
- [coreApiShareOperationsToShares1/gettingShares.feature:39](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiShareOperationsToShares1/gettingShares.feature#L39)

#### [These tests pass in ocis and reva egde but fail in master with `file_target has unexpected value '/home'`](https://github.com/owncloud/ocis/issues/2113)
- [coreApiSharePublicLink1/createPublicLinkShare.feature:306](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharePublicLink1/createPublicLinkShare.feature#L306)
- [coreApiSharePublicLink1/createPublicLinkShare.feature:307](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharePublicLink1/createPublicLinkShare.feature#L307)

#### [valid WebDAV (DELETE, COPY or MOVE) requests with body must exit with 415](https://github.com/owncloud/ocis/issues/4332)
- [coreApiAuthWebDav/webDavDELETEAuth.feature:187](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthWebDav/webDavDELETEAuth.feature#L187)
- [coreApiAuthWebDav/webDavCOPYAuth.feature:137](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiAuthWebDav/webDavCOPYAuth.feature#L137)

#### [PROPFIND on (password protected) public link returns invalid XML](https://github.com/owncloud/ocis/issues/39707)
The problem has been fixed in reva edge branch but not in reva master
- [coreApiWebdavOperations/propfind.feature:61](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/propfind.feature#L61)
- [coreApiWebdavOperations/propfind.feature:73](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiWebdavOperations/propfind.feature#L73)

#### [Updating the role of a public link to internal gives returns 400]
- [coreApiSharePublicLink3/updatePublicLinkShare.feature:496](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharePublicLink3/updatePublicLinkShare.feature#L496)
- [coreApiSharePublicLink3/updatePublicLinkShare.feature:497](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharePublicLink3/updatePublicLinkShare.feature#L497)
- [coreApiSharePublicLink3/updatePublicLinkShare.feature:498](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharePublicLink3/updatePublicLinkShare.feature#L498)
- [coreApiSharePublicLink3/updatePublicLinkShare.feature:499](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiSharePublicLink3/updatePublicLinkShare.feature#L499)

#### [Default capabilities for normal user and admin user not same as in oC-core](https://github.com/owncloud/ocis/issues/1285)
- [coreApiCapabilities/capabilities.feature:8](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiCapabilities/capabilities.feature#L8)
- [coreApiCapabilities/capabilities.feature:133](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiCapabilities/capabilities.feature#L133)
- [coreApiCapabilities/capabilities.feature:172](https://github.com/owncloud/ocis/blob/master/tests/acceptance/features/coreApiCapabilities/capabilities.feature#L172)

Note: always have an empty line at the end of this file.
The bash script that processes this file may not process a scenario reference on the last line.
