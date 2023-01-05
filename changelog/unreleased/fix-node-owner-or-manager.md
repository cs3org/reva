Bugfix: Node owner or manager

DecomposedFS resource info returned the `node.owner` as owner, that is fine in the case of a `personal space`.
But if the current space is a `project space`, the space uuid is used as an owner which makes it impossible to authenticate it.

This is fixed now and the resourceInfo owner returns the owner where it exists and falls back to the first available manager if not.

https://github.com/cs3org/reva/pull/3583
