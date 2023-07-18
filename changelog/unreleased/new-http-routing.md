Enhancement: Centralize HTTP routing

A library has been added handling all the HTTP routing in a common way,
so that each service is only responsible to implement its business logic
and define in a declarative way which are the endpoints and method exposed.
The new routing implementation is based on a radix tree, and supports wildcards:
- `:key`, which is matching a single path segment
- `*key`, which is matching everything

All the existing HTTP services have been migrated to new structure,
but ocdav that requires some more work, and is left as future work.

https://github.com/cs3org/reva/pull/4062
