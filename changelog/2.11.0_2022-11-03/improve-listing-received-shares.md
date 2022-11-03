Bugfix: Improve performance when listing received shares

We improved the performance when listing received shares by getting rid of
superfluous GetPath calls and sending stat request directly to the storage
provider instead of the SharesStorageProvider.

https://github.com/cs3org/reva/pull/3218
