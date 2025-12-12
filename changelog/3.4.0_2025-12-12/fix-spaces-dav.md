Bugfix: Bring back proper DAV support

Spaces broke proper DAV support, because returned hrefs in the PROPFIND always contained space IDs, even if these were not present in the incoming request. This is fixed now, by writing the href based in the incoming URL

https://github.com/cs3org/reva/pull/5409
