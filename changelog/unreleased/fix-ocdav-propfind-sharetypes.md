Bugfix: Only list shares when the PROPFIND actually asks for oc:share-types

`propfindResponse` called `ListPublicShares` and `ListShares` and built the
`usershares`/`linkshares` maps on every PROPFIND, but those maps were only read
by the `oc:share-types` property. That property lives in the explicit-`<d:prop>`
branch and is never returned for `allprop`, so the common `allprop` PROPFIND sent
by the desktop client paid two gRPC round trips per request whose result was
thrown away.

The share lookups are now guarded by a `needsShareTypes` check. On a 1-core
ARMv7 device an `allprop` PROPFIND Depth 0 dropped from 1919 ms to 1370 ms
(-28.6%) and Depth 1 from 2262 ms to 1930 ms (-14.7%). Explicit
`<oc:share-types/>` requests are unchanged.

https://github.com/cs3org/reva/pull/6026
