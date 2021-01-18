Bugfix: Handle removal of public shares by token or ID

Previously different drivers handled removing public shares using different
means, either the token or the ID. Now, both the drivers support both these
methods.

https://github.com/cs3org/reva/pull/1334
