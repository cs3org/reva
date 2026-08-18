Bugfix: resolve /dav/files/<username>/<path> in the right namespace

The home was only applied to the collection root, so everything below it fell
back to the files namespace without the username and every user ended up on the
same path. OPTIONS also 500 because it is not authenticated and had no user to
look a home up for, which stopped clients from mounting the endpoint.

https://github.com/cs3org/reva/pull/5770
