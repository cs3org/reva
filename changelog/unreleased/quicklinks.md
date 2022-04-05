Enhancement: introduced quicklinks

We now support Quicklinks. When creating a link with flag "quicklink=true", no new link will be created when a link
already exists.

Downside: since we can't update cs3api at the moment, we reserve the name `Quicklink` for the quicklink. Trying to create
a link named `Quicklink` without the "quicklink=true" flag will result in a `Bad Request`

https://github.com/cs3org/reva/pull/2715
