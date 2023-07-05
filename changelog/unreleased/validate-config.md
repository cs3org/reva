Enhancement: Enforce/validate configuration of services

Every driver can now specify some validation rules on the
configuration. If the validation rules are not respected,
reva will bail out on startup with a clear error.

https://github.com/cs3org/reva/pull/4035
