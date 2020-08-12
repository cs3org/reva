Bugfix: Use lower-case name for changelog directory

When preparing a new release, the changelog entries need to be copied to the
changelog folder under docs. In a previous change, all these folders were made
to have lower case names, resulting in creation of a separate folder.

https://github.com/cs3org/reva/pull/1025
