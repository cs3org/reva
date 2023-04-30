Enhancement: Prevent overriding mime type mappings in RegisterMime

## Description

This PR updates the **`RegisterMime`** function in the **`mime`** package to prevent overriding mime type mappings. The updated function checks if the given extension already exists in the **`mimes`** map before adding it. If the extension is already registered, the function returns an error. This ensures that existing mime type mappings are not accidentally overwritten.

To implement this change, I added an **`error`** return value to the **`RegisterMime`** function signature and modified the function body to use the **`LoadOrStore`** method of the **`sync.Map`** to atomically check for the existence of the extension in the map and add it if it does not already exist.

https://github.com/cs3org/reva/pull/3833
