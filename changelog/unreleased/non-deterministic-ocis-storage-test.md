Enhancement: Capture non-deterministic behavior on storages 

As a developer creating/maintaining a storage driver I want to be able to validate the atomicity of all my storage driver operations.
* Test for: Start 2 uploads, pause the first one, let the second one finish first, resume the first one at some point in time. Both uploads should finish. Needs to result in 2 versions, last finished is the most recent version.
* Test for: Start 2 MKCOL requests with the same path, one needs to fail.

https://github.com/cs3org/reva/pull/1430
