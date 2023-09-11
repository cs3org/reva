Enhancement: implementation of an app provider for Overleaf

This PR adds an app provider for Overleaf as a standalone http service. 

The app provider currently consists of support for the export to Overleaf 
feature, which when called returns a URL to Overleaf that prompts Overleaf 
to download the appropriate resource making use of the Archiver service, 
and upload the files to a user's Overleaf account.

https://github.com/cs3org/reva/pull/4084
