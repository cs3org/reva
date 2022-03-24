Change: Improve quota handling

GetQuota now returns 0 when no quota was set instead of the disk size.
Also added a new return value for the remaining space which will either be quota - used bytes or if no quota was set the free disk size.

https://github.com/owncloud/ocis/issues/3233
https://github.com/cs3org/reva/pull/2666
