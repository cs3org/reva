Bugfix: Empty meta requests should return body

Meta requests with no resourceID should return a multistatus response body with a 404 part.

https://github.com/cs3org/reva/pull/2899