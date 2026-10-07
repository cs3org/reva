Bugfix: report an error when a COPY or third-party copy upload fails

Both reported success when the upload failed, leaving the destination unwritten,
and a third-party copy push of a missing source panicked instead of returning 404.

https://github.com/cs3org/reva/pull/5774
