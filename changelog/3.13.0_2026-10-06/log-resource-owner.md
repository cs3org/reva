Enhancement: Log who accessed a resource and who owns it

The HTTP log middleware runs outside authentication and sees neither value, so
ocdav records them on the context logger, which already carries the trace id.

https://github.com/cs3org/reva/pull/5859
