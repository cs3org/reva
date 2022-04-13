Bugfix: Use exact match in login filter

After the recent config changes the auth-provider was accidently using a
substring match for the login filter. It's no fixed to use an exact match.

https://github.com/cs3org/reva/pull/2742
