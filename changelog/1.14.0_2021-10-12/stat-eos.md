Bugfix: Fix Stat() for EOS storage provider

This change fixes the convertion between the eosclient.FileInfo
to ResourceInfo, in which the field ArbitraryMetadata was missing.
Moreover, to be consistent with SetArbitraryMetadata() EOS implementation,
all the "user." prefix are stripped out from the xattrs. 

https://github.com/cs3org/reva/pull/2074
