Enhancement: Add support for resource id to the archiver

Before the archiver only supported resources provided by a path.
Now also the resources ID are supported in order to specify the content
of the archive to download. The parameters accepted by the archiver
are two: an optional list of `path` (containing the paths of the
resources) and an optional list of `id` (containing the resources IDs
of the resources).

https://github.com/cs3org/reva/pull/2100
https://github.com/cs3org/reva/issues/2097
