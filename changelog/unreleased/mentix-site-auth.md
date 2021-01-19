Enhancement: Support site authorization status in Mentix

This enhancement adds support for a site authorization status to Mentix. This way, sites registered via a web app can now be excluded until authorized manually by an administrator. 

Furthermore, Mentix now sets the scheme for Prometheus targets. This allows us to also support monitoring of sites that do not support the default HTTPS scheme.

https://github.com/cs3org/reva/pull/1398
