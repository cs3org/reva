Bugfix: Return the updated share after updating 

When updating the state of a share in the in-memory share manager the old share state was returned instead of the updated state.

https://github.com/cs3org/reva/pull/1960
