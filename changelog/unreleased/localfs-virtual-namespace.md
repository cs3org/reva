Bugfix: Fix localhome virtual namespace path handling for spaces

Added optional VirtualHomeTemplate config to localfs driver, enabling localhome
to correctly handle paths when exposing user homes through a virtual namespace
(e.g., /home/<user>) while storing files in a flat per-user layout on disk.

The wrap() function uses a clean switch statement with named predicates to
handle five path transformation patterns:

- Exact match: /home/einstein -> /
- Full path: /home/einstein/file -> /file  
- Parent path: /home -> / (when virtual home is /home/einstein)
- Gateway-stripped parent: /home/file -> /file (gateway omits username)
- Gateway-stripped username: /einstein/file -> /file (WebDAV "home" alias)

The last two cases handle gateway routing edge cases where prefixes are stripped
differently depending on whether the WebDAV layer uses space IDs or the "home"
alias for URL construction.

Unwrap adds the virtual home prefix back (e.g., /file -> /home/einstein/file) to
enable correct space-based WebDAV routing, ensuring PathToSpaceID() derives the
correct space identifier and the UI can construct proper URLs with space IDs.

The localhome wrapper now correctly passes VirtualHomeTemplate through to localfs.

When VirtualHomeTemplate is empty (default), behavior is unchanged, ensuring
backward compatibility with EOS and existing deployments.

https://github.com/cs3org/reva/pull/5404
