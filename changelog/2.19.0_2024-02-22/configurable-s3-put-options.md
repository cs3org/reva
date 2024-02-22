Enhancement: configurable s3 put options

The s3ng blobstore can now be configured with several options: `s3.disable_content_sha254`, `s3.disable_multipart`, `s3.send_content_md5`, `s3.concurrent_stream_parts`, `s3.num_threads` and `s3.part_size`. If unset we default to `s3.send_content_md5: true`, which was hardcoded before. We also default to `s3.concurrent_stream_parts: true` and `s3.num_threads: 4` to allow concurrent uploads even when `s3.send_content_md5` is set to `true`. When tweaking the uploads try setting `s3.send_content_md5: false` and `s3.concurrent_stream_parts: false` first, as this will try to concurrently stream an uploaded file to the s3 store without cutting it into parts first.

https://github.com/cs3org/reva/pull/4526
