Bugfix: OCM-related compatibility fixes

Following analysis of OC and NC code to access a remote share,
we must expose paths and not full URIs on the /ocm-provider endpoint.
Also we fix a lookup issue with apps over OCM and update examples.

https://github.com/cs3org/reva/pull/3962
