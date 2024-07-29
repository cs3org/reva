Bugfix: Always find unique providers

The gateway will now always try to find a single unique provider. It has stopped aggregating multiple ListContainer responses when we removed any business logic from it. 

https://github.com/cs3org/reva/pull/4741
https://github.com/cs3org/reva/pull/4740
https://github.com/cs3org/reva/pull/2394
