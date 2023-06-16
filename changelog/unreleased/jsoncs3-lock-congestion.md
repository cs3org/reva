Bugfix: Reduce jsoncs3 lock congestion

We changed the locking strategy in the jsoncs3 share manager to cause less lock
 congestion increasing the performance in certain scenarios.

https://github.com/cs3org/reva/pull/3985
https://github.com/cs3org/reva/pull/3964
