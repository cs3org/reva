Bugfix: Return 425 on GET

On ocdav GET endpoint the server will now return `425` instead `500` when the file is being processed

https://github.com/cs3org/reva/pull/3688
