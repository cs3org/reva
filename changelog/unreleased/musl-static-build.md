Enhancement: Add musl-based fully static build target

Added a new `revad-static-musl` Makefile target that produces a fully statically
linked binary using musl libc instead of glibc. This eliminates the linker
warnings that appeared with the standard static build and creates a truly portable
binary that runs on any Linux distribution without requiring matching glibc
versions.

Also fixed the build info injection by correcting the package path in BUILD_FLAGS
to include the `/v3` module version, ensuring version, commit, and build date
information are properly displayed in the binary.

https://github.com/cs3org/reva/pull/5407
