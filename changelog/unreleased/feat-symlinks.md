Enhancement: Expose navigable symlink and .lnk targets in PROPFIND

Symbolic links and Windows .lnk shortcuts can now be exposed via two new
`link-type` and `link-target` PROPFIND properties, when explicitly
requested and `enable_link_targets` is set in ocdav. The target href is returned
when the link's target is a relative path that stays within the space (or the
public link) where the link lives, or, for symlinks, when the target is an
absolute path that the gateway resolves for the user to a resource in any
space. Any other link remains an opaque file.
The EOS driver now reports symlinks as such.

https://github.com/cs3org/reva/pull/5868
