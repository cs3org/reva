Change: Fix app provider new file creation and improved error codes

We've fixed the behavior for the app provider when creating new files.
Previously the app provider would overwrite already existing files when creating a new file, this is now handled and prevented.
The new file endpoint accepted a path to a file, but this does not work for spaces. Therefore we now use the resource id of the folder where the file should be created and a filename to create the new file.
Also the app provider returns more useful error codes in a lot of cases.

https://github.com/cs3org/reva/pull/2210
