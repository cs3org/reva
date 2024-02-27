Enhancement: support multiple templates in config entries

This PR introduces support for config entries with multiple templates,
such as `parameter = "{{ vars.v1 }} foo {{ vars.v2 }}"`.
Previously, only one `{{ template }}` was allowed in a given
configuration entry.

https://github.com/cs3org/reva/pull/4282
