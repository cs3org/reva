Bugfix: Take care of trailing slashes in OCM package

Previously, we assumed that the OCM endpoints would have trailing
slashes, failing in case they didn't. This PR fixes that.

https://github.com/cs3org/reva/pull/1024
