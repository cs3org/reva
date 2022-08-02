Bugfix: Do not send body on failed range request

Instead of send the error in the body of a 416 response we log it. This prevents the go reverse proxy from choking on it and turning it into a 502 Bad Gateway response.

https://github.com/cs3org/reva/pull/1884