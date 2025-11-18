Bugfix: Fix localhome space path handling with mount_path stripping

Added optional VirtualHomeTemplate config to localfs driver, allowing localhome
to correctly handle paths when the gateway strips a mount_path prefix. This enables
localhome to expose user homes at /home/<user> (via gateway mount_path="/home")
while storing files in a flat per-user layout on disk.

The driver strips the virtual namespace prefix from incoming paths (e.g., /<user>/file
becomes /file) before prepending the user_layout, and returns storage-relative paths
(e.g., /file) instead of adding the virtual prefix back in unwrap().

The localhome wrapper now passes VirtualHomeTemplate through to localfs.
Parent path handling (e.g., /home when virtual home is /home/einstein) maps to
the authenticated user's root for shared namespace stats.

When VirtualHomeTemplate is empty (default), behavior is unchanged, ensuring
backward compatibility with EOS and existing deployments.

https://github.com/cs3org/reva/pull/5404
