Bugfix: Fix ShareCache concurrency panic

We fixed an issue where concurrently read and write operations led to a panic in the ShareCache. 

https://github.com/cs3org/reva/pull/4887
