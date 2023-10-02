Bugfix: Fix destroying the Personal and Project spaces data

We fixed a bug that caused destroying the Personal and Project spaces data when providing as a destination while move/copy file.
Disallow use the Personal and Project spaces root as a source while move/copy file.

https://github.com/cs3org/reva/pull/4229
https://github.com/owncloud/ocis/issues/6739
