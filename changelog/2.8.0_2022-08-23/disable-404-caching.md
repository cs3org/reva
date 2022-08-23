Bugfix: Disable caching of not found stat responses

We no longer cache not found responses to prevent concurrent requests interfering with put requests.

https://github.com/cs3org/reva/pull/3152
https://github.com/owncloud/ocis/issues/4251
