Bugfix: make stat requests route based on storage providerid

The gateway now uses a filter mask to only fetch the root id of a space for stat requests. This allows the spaces registry to determine the responsible storage provider without querying the storageproviders.

https://github.com/cs3org/reva/pull/2985
