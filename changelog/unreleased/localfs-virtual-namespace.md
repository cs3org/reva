Bugfix: Fix localhome space ID encoding with virtual namespace support

Added optional VirtualHomeTemplate config to localfs driver, allowing localhome
to expose paths in a virtual namespace (e.g., /home/<user>/) while maintaining
the existing filesystem layout. This fixes PathToSpaceID() incorrectly encoding
each file as its own space instead of extracting the correct space root.

The implementation handles parent paths (/home) by mapping them to the authenticated
user's root, enabling spaces registry to stat shared namespace roots correctly.

When VirtualHomeTemplate is empty (default), the original behavior is preserved,
ensuring backward compatibility with EOS and existing deployments.

https://github.com/cs3org/reva/pull/5404
