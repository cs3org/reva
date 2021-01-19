Bugfix: Use the user in request for deciding the layout for non-home DAV requests

For the incoming /dav/files/userID requests, we have different namespaces
depending on whether the request is for the logged-in user's namespace or not.
Since in the storage drivers, we specify the layout depending only on the user
whose resources are to be accessed, this fails when a user wants to access
another user's namespace when the storage provider depends on the logged in
user's namespace. This PR fixes that.

For example, consider the following case. The owncloud fs uses a layout
{{substr 0 1 .Id.OpaqueId}}/{{.Id.OpaqueId}}. The user einstein sends a request
to access a resource shared with him, say /dav/files/marie/abcd, which should be
allowed. However, based on the way we applied the layout, there's no way in
which this can be translated to /m/marie/.

https://github.com/cs3org/reva/pull/1401
