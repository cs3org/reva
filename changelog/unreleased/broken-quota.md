Bugfix: wrong quota total reported

The EOS `QuotaInfo` struct had fields for `AvailableBytes` and `AvailableInodes`, but these were used to mean the total. This is fixed now.

https://github.com/cs3org/reva/pull/5082