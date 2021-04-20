---
title: "atomicity"
linkTitle: "atomicity"
weight: 10
description: >
    Atomicity of DecomposedFS Operations (ocis, s3ng)
---

{{% pageinfo %}}
This document describes the atomicity of (writing) decomposedfs operations by listing the relevant steps that happen
when doing the according operations, highlighting potential problems with concurrent operations and describing the
negative effects.
{{% /pageinfo %}}

## CreateDir
### Steps

1. Check if directory already exists. Abort if it does.
2. Assign a new uuid as the ID
3. Create the node on disk
4. Link the new node to the parent

### Potential Problems

Several concurrent `CreateDir` calls can get past the exit critera step 1 because the directory does not exist yet.
Each of the calls generates a new ID, creates the according node on disk and tries to link it to the parent.
Only the first one succeeds in that, the later ones fail because the link already exists (See
`Considerations > Creating symlinks`).

### Negative Effects

Failing calls will leave an orphaned node behind (See reva issue [#1601](https://github.com/cs3org/reva/issues/1601)).

No risk of inconsistency.

## CreateHome

See `CreateDir`.

## CreateReference

See `CreateDir`.

## Delete
### Steps

1. Get the original path and set it as an xattr on the node
2. Take the current time (with nanosecond precision) and use it to build filename for the deleted file following a defined scheme
3. Create a symlink in the trash directory to the filename in 2 (which doesn't exist yet)
4. Move the file to the destination from step 2
5. Remove the link to the node in the parent

### Potential Problems

There is no exit critera step so all concurrent calls try to create a symlink in step 3 with only one of them 
succeeding (See `Considerations > Creating symlinks`).

### Negative Effects

No risk of inconsistency.


## Move

### Steps

1. Get source node. Abort if it doesn't exist.
2. Get target node. Abort if it exists.
3. Move file.

### Potential Problems

Several concurrent calls can get past the exit criteria steps 1 and 2. But the first writing operation is always the
actual move of the node on the filesystem which is an atomic filesystem operation. That means that with concurrent 
calls only one can ever succeed.

### Negative Effects

None.

## Upload
### Steps

1. Prepare and store internal representation of the upload. This includes storing the current node id if there already is a node for this path.
2. Retrieve and store data in a temporary file
3. Finish upload by uploading the data to the blobstore and writing the node
4. Remove child link in the parent if it exists. Then link the new node to the parent.

### Potential Problems

Retrieving the existing node id happens when the upload is started in step 1. If no node is found at that point in
time a new uuid will be assigned later on in step 3. Making the node visible to other uploads only happens when linking
the node to its parent in step 4 though.

That means that when an upload starts while another one is still running for the same target path they will create
and write different nodes for the same path and both upload the data to the blobstore.

### Negative Effects

With concurrent uploads the last one "wins" by deleting the link from the previous upload and then linking its node in
the owner directory. The others leave orphaned nodes and blobs behind. These uploads seem to have succeeded but their
data is essentially lost, they are *NOT* made available as old versions
(See reva issue [#1626](https://github.com/cs3org/reva/issues/1626)).

## RestoreRevision

### Steps

1. Check if "real" node still exists
2. Move current version to a version file
3. Copy revision file to the "real" node
4. Copy extended attributes (one by one)

### Potential Problems

Moving the file away in step 2 can interfere with concurrent operations.
Another problem exists with step 4 happening concurrently as the different operations overwrite existing
attributes one by one instead of writing the whole set of attributes atomically.

### Negative Effects

Concurrent operations compete with a chance of the others failing ungracefully.
It can even happen that the extended attributes of two revisions are mixed in the resulting node
(See reva issue [#1627](https://github.com/cs3org/reva/issues/1627)).

## RestoreRecycleItem

### Steps

1. Create a link from the restore location to the parent
2. Move the trash item to the restore location
3. Remove the link to the trash item in the trash

### Potential Problems

Only one of the concurrent operations can succeed with step 1 (See `Considerations > Creating symlinks`).
### Negative Effects

None.

## PurgeRecycleItem

### Steps

1. Purge deleted node
2. Delete blob from the blobstore
3. Remove link to deleted node from the trash

### Potential Problems

None.
### Negative Effects

None.

## Considerations

### Creating symlinks

Symlinks are created using the `os.Symlink` function. This function fails if the link already exists. Subsequent
operations are thus guaranteed not to replace a link that has alrady been created.

Example code showing showing this behavior:

```go
package main

import (
	"fmt"
	"io/ioutil"
	"os"
)

func main() {
	err := ioutil.WriteFile("file1", []byte(""), 0600)
	if err != nil {
		os.Exit(1)
	}
	err = ioutil.WriteFile("file2", []byte(""), 0600)
	if err != nil {
		os.Exit(1)
	}

	// Create first symlink
	err = os.Symlink("file1", "link")
	if err != nil {
		os.Exit(1)
	}

	// Try to create symlink, expect EEXISTS
	err = os.Symlink("file2", "link")
	if err == nil {
		os.Exit(1)
	} else {
		fmt.Println(err.Error())
		fmt.Println("Success")
	}
}
```

### Renaming files

Files are renamed using the `os.Rename` function. This function does not fail if it's a file being renamed and the
target already exists. Instead the target is being replaced. Example code:

```go
package main

import (
	"fmt"
	"io/ioutil"
	"os"
)

func main() {
	err := ioutil.WriteFile("file1", []byte(""), 0600)
	if err != nil {
		os.Exit(1)
	}
	err = ioutil.WriteFile("file2", []byte(""), 0600)
	if err != nil {
		os.Exit(1)
	}

	// Overwrite file1 by renaming file2 file, expect no error
	err = os.Rename("file2", "file1")
	if err != nil {
		os.Exit(1)
	} else {
		fmt.Println("Success")
	}
}
```
