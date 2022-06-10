Bugfix: Fix sharing with space ref

We've fixed a bug where share requests with `path` attribute present ignored the `space_ref` attribute. We now give the `space_ref` attribute precedence over the `path` attribute.

https://github.com/cs3org/reva/pull/2950
