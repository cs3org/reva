Enhancement: Share a home folder with lightweight accounts on login

Lightweight accounts now get a home on login: when `lightweight_home_layout`
is set in the gateway, the folder at that path is provisioned by the EOS
`create_lightweight_home_hook` and then shared with the account (as editor)
on behalf of `lightweight_home_owner`, impersonated through machine auth.
