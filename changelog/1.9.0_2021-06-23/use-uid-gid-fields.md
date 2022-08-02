Enhancement: use UidNumber and GidNumber fields in User objects

Update instances where CS3API's `User` objects are created and used to use `GidNumber`,
and `UidNumber` fields instead of storing them in `Opaque` map.

https://github.com/cs3org/reva/issues/1516
