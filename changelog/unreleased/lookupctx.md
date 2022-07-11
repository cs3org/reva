Enhancement: introduce LookupCtx for index interface

The index interface now has a new LookupCtx that can look up multiple values so we can more efficiently look up multiple shares by id.
It also takes a context so we can pass on the trace context to the CS3 backend 

https://github.com/cs3org/reva/pull/3043