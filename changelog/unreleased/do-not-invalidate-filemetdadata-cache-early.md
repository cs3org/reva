Enhancement: Do not invalidate filemetadata cache early

We can postpone overwriting the cache until the metadata has ben written to disk. This prevents other requests trying to read metadata from having to wait for a readlock for the metadata file.

https://github.com/cs3org/reva/pull/4049
