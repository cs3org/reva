#!/bin/bash

# Example script showing how to run NCeph tests with custom directory locations

echo "NCeph Test Directory Configuration Example"
echo "=========================================="
echo

# Example 1: Default behavior (temporary directories)
echo "1. Running tests with default temporary directories:"
echo "   go test -v ./pkg/storage/fs/nceph/..."
echo

# Example 2: Custom test directory
echo "2. Running tests with custom directory (e.g., Ceph mount):"
echo "   export NCEPH_TEST_DIR=/mnt/ceph/reva-tests"
echo "   go test -v ./pkg/storage/fs/nceph/..."
echo

# Example 3: Custom directory with preservation
echo "3. Running tests with custom directory and preservation for debugging:"
echo "   export NCEPH_TEST_DIR=/mnt/ceph/reva-tests"
echo "   export NCEPH_TEST_PRESERVE=true"
echo "   go test -v ./pkg/storage/fs/nceph/..."
echo "   # Test directories will remain after completion"
echo

# Example 4: Running specific test
echo "4. Running a specific test:"
echo "   export NCEPH_TEST_DIR=/mnt/ceph/reva-tests"
echo "   go test -v ./pkg/storage/fs/nceph/ -run TestNCeph_BasicOperations"
echo

# Example 5: Ceph integration tests
echo "5. Running Ceph integration tests (requires real Ceph cluster):"
echo "   # With default Ceph configuration"
echo "   go test -tags ceph -ceph-integration -v ./pkg/storage/fs/nceph/..."
echo
echo "   # With custom Ceph configuration"
echo "   export NCEPH_CEPH_CONFIG=/etc/ceph/custom.conf"
echo "   export NCEPH_CEPH_CLIENT_ID=testuser"
echo "   export NCEPH_CEPH_KEYRING=/etc/ceph/ceph.client.testuser.keyring"
echo "   go test -tags ceph -ceph-integration -v ./pkg/storage/fs/nceph/..."
echo

# Example 6: Running only Ceph integration tests
echo "6. Running specific Ceph integration tests:"
echo "   # Test just the Ceph connection"
echo "   go test -tags ceph -ceph-integration -v ./pkg/storage/fs/nceph/ -run TestCephAdminConnection"
echo
echo "   # Test complete GetPathByID functionality (most comprehensive)"
echo "   go test -tags ceph -ceph-integration -v ./pkg/storage/fs/nceph/ -run TestGetPathByIDIntegration"
echo

# Example 7: What happens when you use the flag incorrectly
echo "7. Incorrect usage (will fail):"
echo "   go test -ceph-integration -v ./pkg/storage/fs/nceph/"
echo "   # This fails because you need -tags ceph"
echo

echo "Environment Variables:"
echo "- NCEPH_TEST_DIR: Base directory for test directories (default: system temp)"
echo "- NCEPH_TEST_PRESERVE: Set to 'true' to keep test directories after completion"
echo
echo "Ceph Configuration Variables (for integration tests):"
echo "- NCEPH_CEPH_CONFIG: Path to ceph.conf (default: /etc/ceph/ceph.conf)"
echo "- NCEPH_CEPH_CLIENT_ID: Ceph client ID (default: admin)"
echo "- NCEPH_CEPH_KEYRING: Path to keyring (default: /etc/ceph/ceph.client.admin.keyring)"
echo

echo "Test Flags:"
echo "- -ceph-integration: Enable tests that require real Ceph cluster connection"
echo "                     (Must be used with -tags ceph)"
echo

echo "Ceph Requirements for GetPathByID tests:"
echo "- MDS admin access: ceph auth caps client.admin mds 'allow *'"
echo "- Without MDS admin access, GetPathByID tests will fail"
echo

echo "For more information, see TESTING.md"
