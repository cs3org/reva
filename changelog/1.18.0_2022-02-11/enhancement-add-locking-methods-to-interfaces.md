Enhancement: add file locking methods to the storage and filesystem interfaces

We've added the file locking methods from the CS3apis to the storage and filesystem
interfaces. As of now they are dummy implementations and will only return "unimplemented"
errors.

https://github.com/cs3org/reva/pull/2350
https://github.com/cs3org/cs3apis/pull/160
