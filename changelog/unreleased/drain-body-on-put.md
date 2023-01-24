Bugfix: Drain body on failed put

When a put request fails the server would not drain the body. This will lead to `connection closed` errors on clients when using http 1

https://github.com/cs3org/reva/pull/3618
