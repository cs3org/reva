Bugfix: Get user from user provider in oidc driver

For oidc providers that only respond with standard claims,
use the user provider to get the user.

https://github.com/cs3org/reva/pull/3055
