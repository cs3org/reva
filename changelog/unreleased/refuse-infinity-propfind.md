Enhancement: refuse propfind with depth infinity

A PROPFIND with Depth: infinity walked the whole tree in one request and
kept all of it in memory. One such request on a project folder took more
than two minutes. Web only uses depth 0 and 1, so ocdav now answers it
with 403, as RFC 4918 allows, for files, spaces and the trash bin. Depth
infinity on COPY and MOVE is not changed.

https://github.com/cs3org/reva/pull/6008/