Enhancement: refactoring of the GORM model for shares

With this PR we introduce new constraints and rename
some fields for better consistency:

* Types used by OCM structures only are prefixed with
  `Ocm`, and `AccessMethod` and `Protocol` were
  consolidated into `OcmProtocol`
* ItemType is used in OCM shares as well
* Unique indexes have been created for all shares
* The `(FileIdPrefix, ItemSource)` tuple is now
  `(StorageId, FileId)` in `OcmShare`, and it was
  removed from `OcmReceivedShare` as unused

https://github.com/cs3org/reva/pull/5402
