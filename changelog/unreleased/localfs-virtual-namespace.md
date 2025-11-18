Bugfix: Fix localhome virtual namespace path handling for spaces

Added optional VirtualHomeTemplate config to localfs driver, enabling localhome
to correctly handle paths when exposing user homes through a virtual namespace
(e.g., /home/<user>) while storing files in a flat per-user layout on disk.

The driver intelligently strips virtual namespace prefixes from incoming paths:

- Full paths: /home/einstein/file -> /file
- Parent paths: /home -> / (when virtual home is /home/einstein)
- Gateway-stripped paths: /home/file -> /file (when gateway omits username)

This last case handles scenarios where the gateway sends paths like /home/Test.txt
instead of /home/einstein/Test.txt, extracting the virtual home parent directory
and stripping it to get the user-relative path.

Unwrap adds the virtual home prefix back (e.g., /file -> /home/file) to enable
correct space-based WebDAV routing, ensuring the UI can construct proper URLs.

The localhome wrapper now correctly passes VirtualHomeTemplate through to localfs.

When VirtualHomeTemplate is empty (default), behavior is unchanged, ensuring
backward compatibility with EOS and existing deployments.

https://github.com/cs3org/reva/pull/5404
