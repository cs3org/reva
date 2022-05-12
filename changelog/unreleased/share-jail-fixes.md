Bugfix: Fix Grant Space IDs and webDAV GET Headers

The opaqueID for a grant space was incorrectly overwritten with the root space id. We fixed that and now use the Filename from the fileinfo in GET WebDAV requests instead of using the value from the url.

https://github.com/cs3org/reva/pull/2864
