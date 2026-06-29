Enhancement: associate on-demand jobs with a user

On-demand background jobs can now be attached to a user through a WithOwner
enqueue option, so the jobs a user created can be listed back (e.g. for a UI)
with ListByOwner; jobs enqueued without it stay internal. An opt-in Unique
option was also added to keep at most one active run per owner and key, so a
user cannot, for instance, start the same export twice.

https://github.com/cs3org/reva/pull/5672
