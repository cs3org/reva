Enhancement: Support OCM v1.0 schema

Following cs3org/cs3apis#206, we add the fields to ensure
backwards compatibility with OCM v1.0. However, if the
`protocol.options` undocumented object is not empty, we bail
out for now. Supporting interoperability with OCM v1.0
implementations (notably Nextcloud 25) may come in the future
if the undocumented options are fully reverse engineered. This
is reflected in the unit tests as well.

Also, added viewMode to webapp protocol options (cs3org/cs3apis#207)
and adapted all SQL code and unit tests.

https://github.com/cs3org/reva/pull/3757
