Enhancement: Memoize the localfs per-user home template expansion

`localfs` re-expanded the per-user layout template with
`templates.WithUser` on every call to `GetHome` / `getVirtualHome`, which a
single directory listing reaches several times per child. The expansion is a
pure function of the template and the user, and the templates are static
config, so it is now memoized per user. This removes ~64 us and 53 KB of
allocation per expansion from every listing.

https://github.com/cs3org/reva/pull/6033
