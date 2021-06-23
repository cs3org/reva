Bugfix: Drone CI - patch the 'store-dev-release' job to fix malformed requests

Replace the backquotes that were used for the date component of the URL with
the POSIX-confirmant command substitution '$()'.

https://github.com/cs3org/reva/pull/1815
