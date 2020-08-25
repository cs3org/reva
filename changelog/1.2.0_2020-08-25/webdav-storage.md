Enhancement: Add logic for resolving storage references over webdav

This PR adds the functionality to resolve webdav references using the ocs
webdav service by passing the resource's owner's token. This would need to be
changed in production.

https://github.com/cs3org/reva/pull/1094
