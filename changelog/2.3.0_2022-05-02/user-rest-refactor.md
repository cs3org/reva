Enhancement: Refactor the rest user and group provider drivers

We now maintain our own cache for all user and group data, and periodically
refresh it. A redis server now becomes a necessary dependency, whereas it was
optional previously.

https://github.com/cs3org/reva/pull/2752