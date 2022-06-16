Change: Do not encode webDAV ids to base64

We removed the base64 encoding of the IDs and use the format <storageID>!<opaqueID> with a `!` delimiter. As a reserved delimiter it is URL safe. The IDs will be XML and JSON encoded as necessary.

https://github.com/cs3org/reva/pull/2559