Enhancement: Run shared invocations on one instance

Invocations that act on shared state, like the jobs store, can be marked `Shared()`. Admin then sends them to one live instance per service instead of to every instance, so `reva admin invoke jobs enqueue` no longer enqueues one run per runner.

https://github.com/cs3org/reva/pull/5924
