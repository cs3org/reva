Bugfix: Fix webdav copy of zero byte files

We've fixed the webdav copy action of zero byte files, which was not performed
because the webdav api assumed, that zero byte uploads are created when initiating
the upload, which was recently removed from all storage drivers. Therefore the 
webdav api also uploads zero byte files after initiating the upload.

https://github.com/cs3org/reva/pull/2374
https://github.com/cs3org/reva/pull/2309
