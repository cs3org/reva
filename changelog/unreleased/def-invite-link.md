Enhancement: Remove redundant config for invite_link_template

This is to drop invite_link_template from the OCM-related config.
The link template is still defined in a constant but not exposed
 in the config, as it depends on Mentix and should not be changed.

https://github.com/cs3org/reva/pull/3905
