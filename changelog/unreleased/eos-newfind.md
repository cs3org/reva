Enhancement: use newfind command in EOS

The EOS binary storage driver was still using EOS's oldfind command, which is deprecated. We now moved to the new find command, for which an extra flag (--skip-version-dirs) is needed.

https://github.com/cs3org/reva/pull/4883