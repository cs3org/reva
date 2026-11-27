Enhancement: Use pure-Go SQLite drivers so CGO_ENABLED=0 builds work

`mattn/go-sqlite3` requires cgo. With `CGO_ENABLED=0` the package still compiled
but embedded a stub, and the first query failed with `go-sqlite3 requires cgo to
work. This is a stub`. Cross-compiling for 32-bit ARM also needed a musl/cgo
toolchain.

`database/sql` users now use `github.com/glebarez/go-sqlite` and the gorm users
use `github.com/glebarez/sqlite`, both of which are pure Go. Static builds for
`GOOS=linux GOARCH=arm GOARM=7` now work without a cross toolchain, and the
resulting revad binary reports no cgo dependency.

https://github.com/cs3org/reva/pull/6032
