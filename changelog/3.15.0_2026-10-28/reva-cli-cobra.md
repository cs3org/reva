Enhancement: rewrite the reva CLI on cobra

The console mode is removed and commands are noun-verb groups with `--long` flags and shell completion.
Settings come from flags, env or `reva config set`, and errors give correct exit codes.

https://github.com/cs3org/reva/pull/5947
