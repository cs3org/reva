Bugfix: use url.JoinPath instead of fmt.Sprintf and path.Join

In some places, we used path.Join instead of url.JoinPath. This leads
to missing slashes in the http prefix, and improper parsing of the URLs.
Instead, we now rely on url.JoinPath

https://github.com/cs3org/reva/pull/5362
