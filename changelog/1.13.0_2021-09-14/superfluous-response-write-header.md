Bugfix: Fix superfluous WriteHeader on file upload 

Removes superfluous Writeheader on file upload and therefore removes the error message "http: superfluous response.WriteHeader call from github.com/cs3org/reva/internal/http/interceptors/log.(*responseLogger).WriteHeader (log.go:154)"

https://github.com/cs3org/reva/pull/2030
