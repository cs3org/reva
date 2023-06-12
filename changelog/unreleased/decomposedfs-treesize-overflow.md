Bugfix: treesize interger overflows

Reading the treesize was parsing the string value as a signed integer while
setting the treesize used unsigned integers this could cause failures (out of
range errors) when reading very large treesizes.

https://github.com/cs3org/reva/pull/3963
