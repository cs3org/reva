Enhancement: Add precondition checks for ocdav trashbin restore

The precondition were not checked before doing a trashbin restore in the ownCloud dav API.
Without the checks the API would behave differently compared to the oC10 API.

https://github.com/cs3org/reva/pull/1795

