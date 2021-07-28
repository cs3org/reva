Enhancement: JWT token mananger now returns the hash of the token

We encode the complete CS3APIs user object along with the scopes the user has
access to in the JWT token. In case the list of scopes is long or the user
belongs to a lot of groups, the token size got pretty big previously, and for
use-cases where we needed to pass it as a URI parameter, led to server limits
on the size of the URI being hit. Now we cache the token and return its hash,
which makes its size constant.

https://github.com/cs3org/reva/pull/1935
