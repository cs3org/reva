Enhancement: add Graph webapp metadata carrier

Graph sharedWithMe items can now carry received webapp metadata as an
`@ocm.webApp` object on `remoteItem`, next to `permissions`, without changing
the generated DriveItem types. Nothing sets the metadata yet, so responses
are unchanged.

https://github.com/cs3org/reva/pull/5852
