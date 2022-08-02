Bugfix: Allow to expose full paths in OCS API

Before this fix a share file_target was always harcoded to use a base path.
This fix provides the possiblity to expose full paths in the OCIS API and asymptotically in OCIS web.

https://github.com/cs3org/reva/pull/1605
