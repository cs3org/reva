Bugfix: look up each project space once per listing

For each entry of a listing, the gateway asked the spaces registry for
the project space, and the registry stat'ed the home and all public
spaces to find it. For a user who is not a project member nothing was
cached. Space roles also gave back share permissions on a space root,
so a project admin could share the root of the project.

https://github.com/cs3org/reva/pull/6006
