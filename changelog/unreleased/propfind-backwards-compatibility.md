Bugfix: Fix propfind response code on forbidden files 

When executing a propfind to a resource owned by another user the service would respond with a HTTP 403.
In ownCloud 10 the response was HTTP 207. This change sets the response code to HTTP 207 to stay backwards compatible.

https://github.com/cs3org/reva/pull/1259

