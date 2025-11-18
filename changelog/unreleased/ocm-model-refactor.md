Enhancement: refactoring of the GORM model for shares

With this PR we introduce new constraints and rename
some fields for better consistency:

* Types used by OCM structures only are prefixed with
  `Ocm`, and `AccessMethod` and `Protocol` were
  consolidated into `OcmProtocol`
* ItemType is used in OCM shares as well
* Unique index constraints have been created as follows:
  * For `Shares` on `(inode, instance, permissions, recipient, deletedAt)`
  * For `OcmShares` on `(storageId, fileId, shareWith, owner, deletedAt)`
* The unique indexes have been renamed with a `u_`
  prefix for consistency: this affected `u_shareid_user`,
  `u_link_token`. The `i_share_with` was dropped
  as redundant.
* The `(FileIdPrefix, ItemSource)` tuple is now
  `(StorageId, FileId)` in `OcmShare`, and it was
  removed from `OcmReceivedShare` as unused

https://github.com/cs3org/reva/pull/5402
