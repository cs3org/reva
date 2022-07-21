Enhancement: skip space lookup on space propfind

We now construct the space id from the /dav/spaces URL intead of making a request to the registry.

https://github.com/cs3org/reva/pull/2977
https://github.com/owncloud/ocis/issues/1277
https://github.com/owncloud/ocis/issues/2144
https://github.com/owncloud/ocis/issues/3073