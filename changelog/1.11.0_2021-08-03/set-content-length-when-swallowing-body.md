Bugfix: Set Content-Length to 0 when swallowing body in the datagateway

When swallowing the body the Content-Lenght needs to be set to 0 to prevent proxies from reading the body.

https://github.com/cs3org/reva/pull/1904