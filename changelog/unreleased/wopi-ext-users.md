Enhancement: use email as display name for external users opening WOPI apps

We use now the email claim for external/federated accounts as the
`username` that is then passed to the wopiserver and used as
`displayName` in the WOPI context.

https://github.com/cs3org/reva/pull/2986
