Enhancement: Set xattrs on files in eosbinary driver

Because in the versions of EOS prior to 4.8.83 all the attrs where lost
when a new version was created, as a workaround these were set on the
version folder. Now, this has been solved and all the attrs will be set
directly on the file.

https://github.com/cs3org/reva/pull/2783