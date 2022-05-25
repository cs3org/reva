Enhancement: stat only after PUT

OCDAV no longer makes a Stat call before making an InitiateFileUpload request. The storageprovider will call GetMD to check if a file exists before initiating the upload and return `"exists"="true"` the opaque data of the InitiateUploadResponse. Storage drivers now should return an Already Exists error when an upload tries to overwrite a directory. 

https://github.com/cs3org/reva/pull/2896
