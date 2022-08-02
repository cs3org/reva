Enhancement: Make the additional info attribute for shares configurable

AdditionalInfoAttribute can be configured via the `additional_info_attribute`
key in the form of a Go template string. If not explicitly set, the default
value is `{{.Mail}}`

https://github.com/cs3org/reva/pull/1588
