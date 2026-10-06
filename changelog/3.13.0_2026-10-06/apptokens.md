Enhancement: implement LoginFlow (apptokens)

Add support for a new login flow, so that different
sync clients can:
- be registered and removed individually
- use a token per sync client, instead of relying on basic auth

This uses NextCloud's /login/v2 flow.

https://github.com/cs3org/reva/pull/5644