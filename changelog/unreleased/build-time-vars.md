Bugfix: Pass build time variables while compiling

We provide the option of viewing various configuration and version options in
both reva CLI as well as the reva daemon, but we didn't actually have these
values in the first place. This PR adds that info at compile time.

https://github.com/cs3org/reva/pull/1069
