Enhancement: implement OCM 1.2

This PR brings in the implementation of parts of OpenCloudMesh 1.2, including:
* Adopting the new properties of the OCM 1.2 payloads, without implementing any new functionality for now. In particular, any non-empty `requirement` in a share will be rejected (a test was added for that).
* Extending the OCM discovery endpoint.
* Using the remote OCM discovery endpoint to establish the full URL of an incoming remote share, regardless if provided or not. When sending a share, though, we still send a full URL.
* Caching the webdav client used to connect to remote endpoints, with added compatibility to OCM 1.0 remote servers.
* Some refactoring and consolidation of duplicated code.
* Improved logging.

https://github.com/cs3org/reva/pull/5076
