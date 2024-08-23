Bugfix: Allow listing directory trash items by key

The storageprovider now passes on the key without inventing a `/` as the relative path when it was not present at the end of the key. This allows differentiating requests that want to get the trash item of a folder itself (where the relative path is empty) or listing the children of a folder in the trash (where the relative path at least starts with a `/`).

We also fixed the `/dav/spaces` endpoint to not invent a `/` at the end of URLs to allow clients to actually make these different requests.

As a byproduct we now return the size of trashed items.

https://github.com/cs3org/reva/pull/4822
https://github.com/cs3org/reva/pull/4818
