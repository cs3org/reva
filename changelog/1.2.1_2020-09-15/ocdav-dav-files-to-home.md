Enhancement: Rewire dav files to the home storage

If the user specified in the dav files URL matches the current one,
rewire it to use the webDavHandler which is wired to the home storage.

This fixes path mapping issues.

https://github.com/cs3org/reva/pull/1125
