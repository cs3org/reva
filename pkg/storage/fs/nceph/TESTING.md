# NCeph Testing with Custom Directory Location

The NCeph tests now support configurable test directory locations through environment variables. This allows you to run tests against a real Ceph mount point instead of temporary directories.

## Environment Variables

### `NCEPH_TEST_DIR`
Set this to specify the base directory where test directories will be created.

**Example:**
```bash
export NCEPH_TEST_DIR=/mnt/ceph/reva-tests
go test -v ./pkg/storage/fs/nceph/...
```

When `NCEPH_TEST_DIR` is set:
- Test directories will be created as subdirectories within this path
- Each test gets its own unique subdirectory (e.g., `/mnt/ceph/reva-tests/nceph_test-1234567890`)
- The base directory will be created if it doesn't exist

### `NCEPH_TEST_PRESERVE`
Set this to `"true"` to preserve test directories after tests complete. This is useful for debugging test failures or inspecting the state after tests run.

**Example:**
```bash
export NCEPH_TEST_DIR=/mnt/ceph/reva-tests
export NCEPH_TEST_PRESERVE=true
go test -v ./pkg/storage/fs/nceph/...
```

### Ceph Configuration Variables

For Ceph integration tests (GetPathByID functionality), you can specify custom Ceph configuration:

- `NCEPH_CEPH_CONFIG`: Path to ceph.conf (default: `/etc/ceph/ceph.conf`)
- `NCEPH_CEPH_CLIENT_ID`: Ceph client ID (default: `admin`)
- `NCEPH_CEPH_KEYRING`: Path to keyring file (default: `/etc/ceph/ceph.client.admin.keyring`)

## Test Types

### Regular Tests
Most tests run without requiring a real Ceph cluster. They test file operations, permissions, chroot functionality, etc.

```bash
go test -v ./pkg/storage/fs/nceph/...
```

### Ceph Integration Tests
Tests that require GetPathByID functionality need a real Ceph cluster connection. These are disabled by default and require both the `ceph` build tag and the `-ceph-integration` flag.

```bash
# Enable Ceph integration tests (requires both build tag and flag)
go test -tags ceph -ceph-integration -v ./pkg/storage/fs/nceph/...
```

**Requirements for Ceph Integration Tests:**
- Ceph build tag: `-tags ceph`
- Integration flag: `-ceph-integration`
- Valid Ceph configuration files
- Network access to a Ceph cluster  
- Proper authentication credentials
- **MDS admin access** for GetPathByID functionality

The tests will automatically skip if:
- The `-ceph-integration` flag is not provided
- Required configuration files are missing

The tests will fail if:
- The `-ceph-integration` flag is provided without `-tags ceph`
- The `-ceph-integration` flag is provided but configuration is invalid
- Configuration exists but connection to the cluster fails
- The Ceph user lacks required MDS admin privileges for GetPathByID functionality

## Usage Examples

### Running tests against a temporary directory (default behavior):
```bash
go test -v ./pkg/storage/fs/nceph/...
```

### Running tests against a Ceph mount:
```bash
export NCEPH_TEST_DIR=/mnt/ceph/reva-tests
go test -v ./pkg/storage/fs/nceph/...
```

### Running tests with preserved directories for debugging:
```bash
export NCEPH_TEST_DIR=/mnt/ceph/reva-tests
export NCEPH_TEST_PRESERVE=true
go test -v ./pkg/storage/fs/nceph/...
# Test directories will remain after tests complete
```

### Running Ceph integration tests with custom configuration:
```bash
export NCEPH_CEPH_CONFIG=/etc/ceph/custom.conf
export NCEPH_CEPH_CLIENT_ID=testuser
export NCEPH_CEPH_KEYRING=/etc/ceph/ceph.client.testuser.keyring
export NCEPH_TEST_DIR=/mnt/ceph/reva-tests
go test -tags ceph -ceph-integration -v ./pkg/storage/fs/nceph/...
```

### Running only specific tests:
```bash
# Regular test
go test -v ./pkg/storage/fs/nceph/ -run TestNCeph_BasicOperations

# Ceph admin connection test (verifies connection only)
go test -tags ceph -ceph-integration -v ./pkg/storage/fs/nceph/ -run TestCephAdminConnection

# Complete GetPathByID integration test (fails on any error)
go test -tags ceph -ceph-integration -v ./pkg/storage/fs/nceph/ -run TestGetPathByIDIntegration
```

## Implementation Details

The implementation adds a test helper file (`test_helper.go`) with the following functions:

### Directory Management
- `GetTestDir(t *testing.T, prefix string) (string, func())`: Creates a test directory and returns a cleanup function
- `SetupTestDir(t *testing.T, prefix string, uid, gid int) (string, func())`: Like GetTestDir but also handles permissions and ownership
- `GetTestSubDir(t *testing.T, baseDir, subDirName string) string`: Creates subdirectories within a test directory

### Ceph Integration Testing
- `RequireCephIntegration(t *testing.T)`: Checks for the `-ceph-integration` flag and validates Ceph configuration
- `ValidateCephConfig(t *testing.T) bool`: Validates that required Ceph configuration files exist
- `GetCephConfig() map[string]interface{}`: Returns Ceph configuration from environment variables or defaults

All existing test functions have been updated to use these helpers instead of `os.MkdirTemp()`.

## Benefits

1. **Real Ceph Testing**: Run tests against actual Ceph filesystems to catch integration issues
2. **Selective Integration Testing**: Only run expensive Ceph tests when explicitly requested
3. **Configuration Validation**: Tests fail fast if Ceph configuration is incomplete
4. **Debugging**: Preserve test directories to inspect filesystem state after test failures
5. **Consistency**: All tests use the same directory setup mechanism
6. **Backward Compatibility**: Tests still work without any environment variables set

## Notes

- When using a custom test directory, ensure the location has appropriate permissions
- The test user needs write access to the specified directory
- Test directories are created with appropriate permissions automatically
- Each test run gets unique subdirectories to avoid conflicts
- Ceph integration tests require network access to a Ceph cluster
- Invalid Ceph configuration will cause integration tests to fail (not skip)
- MDS admin access is required for GetPathByID tests to pass

## Ceph MDS Admin Access

GetPathByID functionality requires MDS (Metadata Server) admin access. To grant this:

```bash
# Grant MDS admin capabilities to your client user
ceph auth caps client.admin mds 'allow *' mon 'allow *' osd 'allow *'

# Or create a specific user with MDS admin access
ceph auth get-or-create client.reva-test mds 'allow *' mon 'allow r' osd 'allow rw'

# Verify the user has the correct capabilities
ceph auth get client.admin
```

If your Ceph user lacks MDS admin access, the GetPathByID integration tests will fail with an error like:
```
GetPathByID failed. This indicates issues with Ceph connection, privileges, or configuration: 
nceph: failed to get path by inode: error: not supported: nceph: inode to path resolution requires MDS admin access
```

## Test Structure

The Ceph integration tests are split into two focused tests:

1. **`TestCephAdminConnection`** - Verifies that a Ceph admin connection can be established
   - Tests basic connectivity and authentication
   - Fails if connection cannot be established

2. **`TestGetPathByIDIntegration`** - Tests the complete GetPathByID functionality
   - Requires working Ceph connection AND proper MDS admin privileges
   - Fails on ANY error (connection, privileges, file not found, etc.)
   - This is the most comprehensive test for Ceph integration
