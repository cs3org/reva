Change: Different type of references

We unified the `Reference_Id` end `Reference_Path` types to a combined `Reference` that contains both:
- id based properties that can identify a node using a `storage_id` and `node_id`
- a `path` property that can be used to represent absolute paths as well as paths relative to the id based properties.
While this is a breaking change it allows more flexible passing of references.

https://github.com/cs3org/reva/pull/1721