Bugfix: Fix several problems in the cephmount user threads

- Cleanup stopped a user thread while a long request still ran on it.
- A request whose caller had stopped waiting still ran later. 
- An upload copied its data on the user thread, so a slow client blocked the other requests of that user. 
- Uploads now also return the error from closing the file.
- A data race on the thread ID is also fixed.

https://github.com/cs3org/reva/pull/5969