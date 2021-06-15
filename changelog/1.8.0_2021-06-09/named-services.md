Enhancement: Named Service Registration

move away from hardcoding service IP addresses and rely upon name resolution instead. It delegates the address lookup to a static in-memory service registry, which can be re-implemented in multiple forms.

https://github.com/cs3org/reva/pull/1509