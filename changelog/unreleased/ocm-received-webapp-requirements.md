Bugfix: validate received webapp requirements

Incoming OCM webapp shares now require `must-exchange-token`, reject blank,
padded, and unknown requirements, and refuse `must-use-mfa` unless
`mfa_policy` is `off`, the same setting the launch path uses. A webapp share is
stored only after sender discovery reports `exchange-token` with a usable
token endpoint and the webapp URI is absolute http(s); relative URIs are
rejected. OCM discovery now advertises `apiVersion` 1.4.0.

The SQL store now keeps webapp permissions exact.

https://github.com/cs3org/reva/pull/5855
