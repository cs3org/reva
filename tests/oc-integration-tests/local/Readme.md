# Local deployment of reva

## Notable Services
`frontend-global.toml` serves all HTTP services on 20180 with a global namespace on the `/webdav` and `/dav/users/{username}` endpoints. This mimics the cernbox deployment.
`frontend.toml` serves all HTTP services on 20080, jailing users into their home on the `/webdav` and `/dav/users/{username}` endpoints. This mimics the classic ownCloud.

Use either `users.toml` or `users-ldap.toml`. You cannot use both at the same time.