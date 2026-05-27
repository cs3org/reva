Bugfix: throw proper err upon invalid COPY request

When doing a COPY / MOVE with an invalid space id,
Reva starts copying from `/` instead of returning an error

https://github.com/cs3org/reva/pull/5624