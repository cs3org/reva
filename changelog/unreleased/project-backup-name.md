Enhancement: replace backup_job_id with backup_name in projects table

The backup_name column stores the project name plus a salt, used to identify the project in the backup system.

https://github.com/cs3org/reva/pull/5771
