Bugfix: keep lock structs in a local map protected by a mutex

Make sure that only one go routine or process can get the 
lock.

https://github.com/cs3org/reva/pull/2582
