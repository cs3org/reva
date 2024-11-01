Bugfix: PurgeDate in ListDeletedEntries was ignored

The date range that can be passed to ListDeletedEntries was not taken into account due to a bug in reva: the Purgedate argument was set, which only works for PURGE requests, and not for LIST requests. Instead, the Listflag argument must be used. Additionally, there was a bug in the loop that is used to iterate over all days in the date range.

https://github.com/cs3org/reva/pull/4905
