Enhancement: expose received webapp metadata

Graph sharedWithMe now returns `remoteItem["@ocm.webApp"].appName` for a
received OCM share that carries exactly one webapp protocol. The stored name
is returned as is, including an empty string. Shares with no webapp protocol,
or with more than one, omit the key.

https://github.com/cs3org/reva/pull/5853
