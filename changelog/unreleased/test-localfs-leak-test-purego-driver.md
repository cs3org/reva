Enhancement: Drive the localfs leak test with the pure-Go sqlite driver

The regression tests added in #6035 wrap mattn/go-sqlite3 to count Prepare and
Query calls. That driver compiles to a stub when CGO_ENABLED=0, so with the
pure-Go driver from #6032 in use the tests fail at run time with
"go-sqlite3 requires cgo to work" instead of at build time. mattn/go-sqlite3
stays in go.mod as an indirect dependency, so nothing fails to link and a
CGO-enabled CI run does not notice.

The tests now wrap github.com/glebarez/go-sqlite, the driver #6032 uses for
localfs, so they run in both build modes. Detection is unchanged: removing the
deferred statement or rows closes still fails the tests.

https://github.com/cs3org/reva/pull/6048
