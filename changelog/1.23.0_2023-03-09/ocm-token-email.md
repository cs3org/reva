Enhancement: Specify recipient as a query param when sending OCM token by email

Before the email recipient when sending the OCM token was specified as a form parameter.
Now as a query parameter, as some clients does not allow in a GET request to set form values.
It also add the possibility to specify a template for the subject and the body for the token email.

https://github.com/cs3org/reva/pull/3687