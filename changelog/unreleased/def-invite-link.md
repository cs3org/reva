Enhancement: Remove redundant config for invite_link_template

This is to drop invite_link_template from the OCM-related config.
Now the provider_domain and mesh_directory_url config options
are both mandatory in the sciencemesh http service, and the link
is directly built out of the context.

https://github.com/cs3org/reva/pull/3905
