Enhancement: Create a home for lightweight accounts on login

When `lightweight_home_layout` is set in the gateway, a lightweight account
that cannot reach the folder at that path on login triggers `CreateHome` on
the storage provider holding it. The EOS driver then runs
`create_lightweight_home_hook`, which creates the folder and shares it with
the account.

https://github.com/cs3org/reva/pull/5843/