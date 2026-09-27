Bugfix: validate received webapp requirements

Incoming OCM webapp shares now require `must-exchange-token`, reject blank,
padded, and unknown requirements, and refuse `must-use-mfa`. A webapp share is
stored only after sender discovery reports `exchange-token` with a usable
token endpoint and the webapp URI is absolute http(s); relative URIs are
rejected. OCM discovery now advertises `apiVersion` 1.4.0.

https://github.com/cs3org/reva/pull/5855
