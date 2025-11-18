Bugfix: Fix localhome space ID encoding with virtual namespace support

Added optional VirtualHomeTemplate config to localfs driver, allowing localhome
to expose paths in a virtual namespace (e.g., /home/<user>/) while maintaining
the existing filesystem layout. This fixes PathToSpaceID() incorrectly encoding
each file as its own space instead of extracting the correct space root.

When VirtualHomeTemplate is empty (default), the original behavior is preserved,
ensuring backward compatibility with EOS and existing deployments.
