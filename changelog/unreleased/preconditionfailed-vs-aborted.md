Enhancement: distinguish GRPC FAILED_PRECONDITION and ABORTED codes

Webdav distinguishes between 412 precondition failed for if match errors for locks or etags, uses 405 Method Not Allowed when trying to MKCOL an already existing collection and 409 Conflict when intermediate collections are missing.

The CS3 GRPC status codes are modeled after https://github.com/googleapis/googleapis/blob/master/google/rpc/code.proto. When trying to use the error codes to distinguish these cases on a storageprovider CreateDir call we can map ALREADY_EXISTS to 405, FAILED_PRECONDITION to 409 and ABORTED to 412.

Unfortunately, we currently use and map FAILED_PRECONDITION to 412. I assume because the naming is very similar to PreconditionFailed. However the GRPC docs ar very clear that ABORTED should be used, specifically mentioning etas and locks.

With this PR we internally clean up the usage in the decompesedfs and mapping in the ocdav handler.

https://github.com/cs3org/reva/pull/3003
