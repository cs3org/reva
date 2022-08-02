Enhancement: Cache whether a user home was created or not

Previously, on every call, we used to stat the user home to make sure that it
existed. Now we cache it for a given amount of time so as to avoid repeated
calls.

https://github.com/cs3org/reva/pull/2248