Bugfix: Add missing domain normalization to mentix provider authorizer

The Mentix OCM Provider authorizer lacked provider domain normalization.
This led to incorrect provider domain matching when authorizing OCM providers.

https://github.com/cs3org/reva/pull/3121
