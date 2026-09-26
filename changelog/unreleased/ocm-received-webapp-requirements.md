Bugfix: reject unsafe received webapp requirements during ingest

Incoming webapp shares now use one requirement policy. must-exchange-token
is mandatory, blank, padded, and unknown requirements fail, and
must-use-mfa is permanently rejected. A webapp share is stored only
after discovery reports exchange-token and a usable token endpoint,
and only when the webapp URI is already absolute http or https. Relative
webapp URIs are not resolved. Discovery advertises apiVersion 1.4.0 to
every client. That version is not a claim of full OCM specification
compliance. enable_webapp only controls discovery and stays distinct from
the outbound offer gate. Invalid or empty discovery bases stay disabled and
publish no receive targets.
