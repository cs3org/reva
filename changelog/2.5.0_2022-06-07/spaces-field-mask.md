Enhancement: introduce spaces field mask

We now use a field mask to select which properties to retrieve when looking up storage spaces. This allows the gateway to only ask for `root` when trying to forward id or path based requests.

https://github.com/cs3org/reva/pull/2888