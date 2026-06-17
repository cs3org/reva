Enhancement: Implement the new OCM webapp protocol

Following the OCM specifications, the webapp protocol and access method
were reworked in the cs3apis: the view mode was replaced by share
permissions (view, read, write, share on the wire), and the protocol now
carries a shared secret, requirements (including must-exchange-token),
targets (blank or iframe), and optional display metadata (appName,
appIconHint, mediaTypes).

- The OCM /shares endpoint now validates and parses webapp protocol
  payloads according to the new specification; legacy payloads carrying
  a viewMode are rejected
- Outgoing webapp shares are serialized with the new wire fields, with
  the shared secret taken from the share token
- The SQL share manager persists the new webapp fields
- Roles for webapp-only shares are now derived from the share
  permissions instead of the view mode

https://github.com/cs3org/reva/pull/5664
