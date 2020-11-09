Enhancement: Add logic in EOS FS for maintaining same inode across file versions

This PR adds the functionality to maintain the same inode across various
versions of a file by returning the inode of the version folder which remains
constant. It requires extra metadata operations so a flag is provided to disable
it.

https://github.com/cs3org/reva/pull/1174
