Enhancement: Increase trashbin restore API compatibility

* The precondition were not checked before doing a trashbin restore in the ownCloud dav API. Without the checks the API would behave differently compared to the oC10 API.
* The restore response was missing HTTP headers like `ETag`
* Update the name when restoring the file from trashbin to a new target name

https://github.com/cs3org/reva/pull/1795

