Bugfix: Fix ocis move

When renaming a file we updating the name attribute on the wrong node, causing the path construction to use the wrong name. This fixes the litmus move_coll test.

https://github.com/cs3org/reva/pull/1177