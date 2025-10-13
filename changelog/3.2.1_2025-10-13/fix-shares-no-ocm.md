Bugfix: sharedWithMe without OCM

If OCM is not enabled (i.e. the drivers are not there), then currently `sharedWithMe`
and `sharedByMe` would fail. We now only log errors from OCM and still provide a response.

https://github.com/cs3org/reva/pull/5326
