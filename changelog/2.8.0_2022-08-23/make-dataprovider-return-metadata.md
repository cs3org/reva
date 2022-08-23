Change: dataproviders now return file metadata

Dataprovider drivers can now return file metadata. When the resource info contains a file id, the mtime or an etag, these will be included in the response as the corresponding http headers.

https://github.com/cs3org/reva/pull/3154
