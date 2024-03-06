Bugfix: fixed registration of custom extensions in the mime registry

This PR ensures custom extensions/mime-types are registered by trimming
any eventual leading '.' from the extension.

https://github.com/cs3org/reva/pull/4319
