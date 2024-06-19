Bugfix: Duplicate headers in DAV responses

We fixed an issue where the DAV response headers were duplicated. This was caused by the WebDav handler which copied over all headers from the datagateways response. Now, only the relevant headers are copied over to the DAV response to prevent duplication.

https://github.com/cs3org/reva/pull/4711
