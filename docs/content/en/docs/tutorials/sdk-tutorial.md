---
title: "Reva SDK"
linkTitle: "Reva SDK"
weight: 5
description: >
  Use the Reva SDK to easily access and work with a remote Reva instance. 
---
The Reva SDK (located under `/pkg/sdk/`) is a simple software development kit to work with Reva through the [CS3API](https://github.com/cs3org/go-cs3apis). It's goal is to make working with Reva as easy as possible by providing a high-level API which hides all the details of the underlying CS3API.

## Design
The main design goal of the SDK is _simplicity_. This means that the code is extremely easy to use instead of being particularly fancy or feature-rich. 

There are two central kinds of objects you'll be using: a _session_ and various _actions_. The session represents the connection to the Reva server through its gRPC gateway client; the actions are then used to perform operations like up- and downloading or enumerating all files located at a specific remote path.

## Using the SDK
### 1. Session creation
The first step when using the SDK is to create a session and establish a connection to Reva (which actually results in a token-creation and not a permanent connection, but this should not bother you in any way):

```
session := sdk.MustNewSession() // Panics if this fails (should usually not happen)
session.Initiate("reva.host.com:443", false)
session.BasicLogin("my-login", "my-pass")
```

Note that error checking is omitted here for brevity, but nearly all methods in the SDK return an error which should be checked upon.

If the session has been created successfully - which can also be verified by calling `session.IsValid()` -, you can use one of the various actions to perform the actual operations.

### 2. Performing operations
An overview of all currently supported operations can be found below; here is an example of how to upload a file using the `UploadAction`:

```
act := action.MustNewUploadAction(session)
info, err := act.UploadBytes([]byte("HELLO WORLD!\n"), "/home/mytest/hello.txt")
// Check error...
fmt.Printf("Uploaded file: %s [%db] -- %s", info.Path, info.Size, info.Type)
```

As you can see, you first need to create an instance of the desired action by either calling its corresponding `New...Action` or `MustNew...Action` function; these creators always require you to pass the previously created session object. The actual operations are then performed by using the appropriate methods offered by the action object. 

A more extensive example of how to use the SDK can also be found in `/examples/sdk/sdk.go`.

## Supported operations
An action object often bundles various operations; the `FileOperationsAction`, for example, allows you to create directories, check if a file exists or remove an entire path. Below is an alphabetically sorted table of the available actions and their supported operations:

| Action | Operation | Description |
| --- | --- | --- |
| `DownloadAction` | `Download` | Downloads a specific resource identified by a `ResourceInfo` object |
|  | `DownloadFile` | Downloads a specific file |
| `EnumFilesAction`<sup>1</sup> | `ListAll` | Lists all files and directories in a given path |
| | `ListAllWithFilter` | Lists all files and directories in a given path that fulfill a given predicate |
| | `ListDirs` | Lists all directories in a given path |
| | `ListFiles` | Lists all files in a given path |
| `FileOperationsAction` | `DirExists` | Checks whether the specified directory exists |
| | `FileExists` | Checks whether the specified file exists |
| | `MakePath` | Creates the entire directory tree specified by a path |
| | `Move` | Moves a specified resource to a new target |
| | `MoveTo` | Moves a specified resource to a new directory, creating it if necessary |
| | `Remove` | Deletes the specified resource |
| | `ResourceExists` | Checks whether the specified resource exists |
| | `Stat` | Queries information of a resource |
| `UploadAction`<sup>2</sup> | `Upload` | Uploads data from a reader to a target file |
| | `UploadBytes` | Uploads byte data to a target file |
| | `UploadFile` | Uploads a file to a target file |
| | `UploadFileTo` | Uploads a file to a target directory |  

* <sup>1</sup> All enumeration operations support recursion.
* <sup>2</sup> The `UploadAction` creates the target directory automatically if necessary.

_Note that not all features of the CS3API are currently implemented._ 
