Enhancement: New configuration

Allow multiple driverts of the same service to be in the
same toml config. Add a `vars` section to contain common
parameters addressable using templates in the configuration
of the different drivers. Support templating to reference
values of other parameters in the configuration.
Assign random ports to services where the address is not
specified.

https://github.com/cs3org/reva/pull/4015
