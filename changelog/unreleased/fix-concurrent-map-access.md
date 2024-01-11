Bugfix: Fix concurrent map access in sharecache

We fixed a problem where the sharecache map would sometimes cause a panic when being accessed concurrently.

https://github.com/cs3org/reva/pull/4457
