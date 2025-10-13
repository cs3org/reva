Bugfix: slash missing in public links

We used `path.Join` from the standard library for constructing URLs in two
cases, which is wrong, as this will remove double slashes, leading to
`https:/...`. This has now been fixed.

https://github.com/cs3org/reva/pull/5361
