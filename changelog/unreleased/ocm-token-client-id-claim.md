Bugfix: add client_id claim to minted OCM token

The OCM token endpoint now accepts only the `authorization_code` grant, and
the form `client_id` must be the receiving provider FQDN and match the stored
share recipient. Tokens minted through the code flow carry a `client_id`
claim set to the share id; legacy direct-secret tokens do not.

https://github.com/cs3org/reva/pull/5856
