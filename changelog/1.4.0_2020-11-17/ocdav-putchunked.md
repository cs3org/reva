Bugfix: Upload file to storage provider after assembling chunks

In the PUT handler for chunked uploads in ocdav, we store the individual
chunks in temporary file but do not write the assembled file to storage.
This PR fixes that.

https://github.com/cs3org/reva/pull/1253
