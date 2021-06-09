Bugfix: set Content-Type header correctly for ocs requests

Before this fix the `Content-Type` header was guessed by `w.Write` because `WriteHeader` was called to early. Now the `Content-Type` is set correctly and to the same values as in ownCloud 10

https://github.com/owncloud/ocis/issues/1779
