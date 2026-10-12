Enhancement: scope app passwords to permissions, roots and a client kind

The new `apppassword` scope limits a token to a catalogue of operations, optional root folders and a kind of client, checked in the HTTP auth middleware and in every gRPC service.
Nothing mints it yet: existing app passwords keep the owner scope.

https://github.com/cs3org/reva/pull/5894
