Enhancement: refactoring of the GORM model for shares

With this PR we introduce new constraints and rename
some fields for better consistency:

* Types used by OCM structures only are prefixed with
  `Ocm`, and `AccessMethod` and `Protocol` were
  consolidated into `OcmProtocol`
* ItemType is used in OCM shares as well
* The `(FileIdPrefix, ItemSource)` tuple is now
  `(Instance, Inode)` in `OcmShare`, and it was
  removed from `OcmReceivedShare` as unused
* Unique index constraints have been created for regular `Shares`
and for `OcmShares` on `(instance, inode, shareWith, deletedAt)`
* The unique indexes have been renamed with a `u_`
  prefix for consistency: this affected `u_shareid_user`,
  `u_link_token`. The `i_share_with` was dropped
  as redundant.
* `Alias` and `Hidden` were added in `OcmReceivedShare`

https://github.com/cs3org/reva/pull/5402
