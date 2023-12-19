Bugfix: Fix disconnected traces

We fixed a problem where the appctx logger was using a new traceid instead of picking up the one from the trace parent.

https://github.com/cs3org/reva/pull/4422
