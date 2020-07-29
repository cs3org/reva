Enhancement: Pass the link to the meshdirectory service in token mail

Currently, we just forward the token and the original user's domain when
forwarding an OCM invite token and expect the user to frame the forward invite
URL. This PR instead passes the link to the meshdirectory service, from where
the user can pick the provider they want to accept the invite with.

https://github.com/cs3org/reva/pull/1002
https://github.com/sciencemesh/sciencemesh/issues/139
