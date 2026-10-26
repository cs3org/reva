Enhancement: give the reva CLI the command names of the cernbox CLI

The reva CLI and the cernbox CLI will become one program, so the reva CLI
takes the names of the cernbox CLI before its first release:
`public-share` is `link`, `recycle` is `trash` (restore and purge take several
keys), `app-password` is `token create|list|revoke ID`, `upload` and `download`
are `cp SOURCE DEST` with `cb:` on the remote side, and `transfer` is
`ocm transfer`. OCM has `invite create|list|accept`, `contacts`, `providers`
and `received`. `login --method`, `logout`, `status` and an own `completion`
command are new.

The global flags are `--gateway` (was `--host`), `--token`,
`--app-token-file` with `--user` (in place of `--token-file`), `-q`,
`--debug`, `--no-progress` and `-o table|json`. The settings are in
`$XDG_CONFIG_HOME/reva/config.toml`, and `~/.reva.config` is copied there once.

For the cernbox CLI, the JSON output will become the CS3 protojson of each
resource when it moves to this code.

https://github.com/cs3org/reva/pull/XXXX
