Bugfix: Revert and adapt touch on office file create

We have reverted https://github.com/cs3org/reva/pull/4959 and
instead implemented added a touch file bevore the file is uploaded.

https://github.com/cs3org/reva/pull/4962
https://github.com/owncloud/ocis/issues/8950