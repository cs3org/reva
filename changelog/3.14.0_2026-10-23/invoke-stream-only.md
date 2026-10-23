Enhancement: stream a stream-only invocation without -stream

`reva admin invoke` now streams an invocation that has only a streaming form. The invocation spec gets `stream_only` for this, and `-stream` is still needed to follow an invocation with both forms, like `logs`.

https://github.com/cs3org/reva/pull/5944
