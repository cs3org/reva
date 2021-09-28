Bugfix: Add as default app while registering and skip unset mimetypes

We fixed that app providers will be set as default app while registering if configured.
Also we changed the behaviour that listing supported mimetypes only displays allowed / configured mimetypes.

https://github.com/cs3org/reva/pull/2114
https://github.com/cs3org/reva/pull/2095
