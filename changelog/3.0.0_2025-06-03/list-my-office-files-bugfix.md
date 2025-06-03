Bugfix: ListMyOfficeFiles

There was a bug in the regex for excel files: 
"(.*?)(.xls|.XLS|)[x|X]?$" contains an extra "|"

https://github.com/cs3org/reva/pull/5148