Enhancement: implemented folderurl for WOPI apps

The folderurl is now populated for WOPI apps, such that
for owners and named shares it points to the containing
folder, and for public links it points to the appropriate
public link URL.

On the way, functions to manipulate the user's scope and
extract the eventual public link token(s) have been added,
coauthored with @gmgigi96.

https://github.com/cs3org/reva/pull/3494
