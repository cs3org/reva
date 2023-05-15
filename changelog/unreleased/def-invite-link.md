Enhancement: Remove config for invite_link

This is to drop invite_link from the OCM-related config.

The link template is still defined in a constant but not exposed
 in the config, as it depends on Mentix and should not be changed.

https://github.com/cs3org/reva/pull/3905
