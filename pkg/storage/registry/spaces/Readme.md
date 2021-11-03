Spaces Registry
===============


The spaces registry recognizes individual spaces instead of strorego providers.
While it is configured with a list of storage providers, it will query them for all storage spaces and use the space ids to resolve id based lookups.
Furthermore, path based lookups will take into account space type and name to present a human readable file tree.

TODO deal with name collisions ... höhö