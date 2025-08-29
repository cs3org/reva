// Copyright 2018-2024 CERN
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// In applying this license, CERN does not waive the privileges and immunities
// granted to it by virtue of its status as an Intergovernmental Organization
// or submit itself to any jurisdiction.

//go:build linux

package nceph

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"sync"
	"syscall"
	"time"

	userv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
)

// PrivilegeVerificationResult contains the results of privilege verification
type PrivilegeVerificationResult struct {
	CanChangeUID    bool
	CanChangeGID    bool
	CurrentUID      int
	CurrentGID      int
	CurrentFsUID    int
	CurrentFsGID    int
	TestedUIDs      []int
	TestedGIDs      []int
	ErrorMessages   []string
	Recommendations []string
}

// VerifyPrivileges checks if the current process has sufficient privileges to use setfsuid/setfsgid
func VerifyPrivileges(nobodyUID, nobodyGID int) *PrivilegeVerificationResult {
	result := &PrivilegeVerificationResult{
		CurrentUID:   os.Getuid(),
		CurrentGID:   os.Getgid(),
		CurrentFsUID: setfsuidSafe(-1), // Get current fsuid without changing it
		CurrentFsGID: setfsgidSafe(-1), // Get current fsgid without changing it
		TestedUIDs:   []int{},
		TestedGIDs:   []int{},
	}

	// Test UID changes - only test meaningful changes (not current UID)
	testUIDs := []int{}
	if nobodyUID != result.CurrentUID {
		testUIDs = append(testUIDs, nobodyUID)
	}
	if result.CurrentUID != 0 {
		testUIDs = append(testUIDs, 0) // Test root if we're not root
	}
	if nobodyUID != 65534 && result.CurrentUID != 65534 {
		testUIDs = append(testUIDs, 65534) // Test standard nobody if different
	}

	// We can always "change" to our current UID, so set this as baseline
	result.CanChangeUID = true

	for _, uid := range testUIDs {
		originalFsuid := setfsuidSafe(-1) // Get current fsuid before test
		oldUID := setfsuidSafe(uid)       // Change to test UID
		actualUID := setfsuidSafe(-1)     // Get current value without changing

		// Restore original value IMMEDIATELY
		setfsuidSafe(oldUID)
		finalUID := setfsuidSafe(-1) // Verify restoration

		result.TestedUIDs = append(result.TestedUIDs, uid)
		if actualUID != uid {
			// If we couldn't change to this UID, we don't have full privileges
			result.CanChangeUID = false
		}
		
		// Log verbose information about privilege test and restoration
		if originalFsuid != finalUID {
			// This should not happen - log a warning
			result.ErrorMessages = append(result.ErrorMessages, 
				fmt.Sprintf("UID restoration failed: original=%d, final=%d after testing UID %d", 
					originalFsuid, finalUID, uid))
		}
	}

	// Test GID changes - only test meaningful changes (not current GID)
	testGIDs := []int{}
	if nobodyGID != result.CurrentGID {
		testGIDs = append(testGIDs, nobodyGID)
	}
	if result.CurrentGID != 0 {
		testGIDs = append(testGIDs, 0) // Test root if we're not root
	}
	if nobodyGID != 65534 && result.CurrentGID != 65534 {
		testGIDs = append(testGIDs, 65534) // Test standard nobody if different
	}

	// We can always "change" to our current GID, so set this as baseline
	result.CanChangeGID = true

	for _, gid := range testGIDs {
		originalFsgid := setfsgidSafe(-1) // Get current fsgid before test
		oldGID := setfsgidSafe(gid)       // Change to test GID
		actualGID := setfsgidSafe(-1)     // Get current value without changing

		// Restore original value IMMEDIATELY
		setfsgidSafe(oldGID)
		finalGID := setfsgidSafe(-1) // Verify restoration

		result.TestedGIDs = append(result.TestedGIDs, gid)
		if actualGID != gid {
			// If we couldn't change to this GID, we don't have full privileges
			result.CanChangeGID = false
		}
		
		// Log verbose information about privilege test and restoration
		if originalFsgid != finalGID {
			// This should not happen - log a warning
			result.ErrorMessages = append(result.ErrorMessages, 
				fmt.Sprintf("GID restoration failed: original=%d, final=%d after testing GID %d", 
					originalFsgid, finalGID, gid))
		}
	}

	// Generate error messages and recommendations
	result.generateMessages()

	return result
}

// generateMessages creates error messages and recommendations based on test results
func (r *PrivilegeVerificationResult) generateMessages() {
	if !r.CanChangeUID && !r.CanChangeGID {
		r.ErrorMessages = append(r.ErrorMessages, "Cannot change filesystem UID or GID - insufficient privileges")
		r.Recommendations = append(r.Recommendations, "Run with elevated privileges (e.g., sudo) or configure capabilities")
	} else if !r.CanChangeUID {
		r.ErrorMessages = append(r.ErrorMessages, "Cannot change filesystem UID - insufficient privileges")
		r.Recommendations = append(r.Recommendations, "Process needs CAP_SETUID capability or root privileges")
	} else if !r.CanChangeGID {
		r.ErrorMessages = append(r.ErrorMessages, "Cannot change filesystem GID - insufficient privileges")
		r.Recommendations = append(r.Recommendations, "Process needs CAP_SETGID capability or root privileges")
	}

	if r.CurrentUID != 0 {
		r.Recommendations = append(r.Recommendations, "Consider running as root or with appropriate capabilities")
		r.Recommendations = append(r.Recommendations, "Alternative: Use 'setcap cap_setuid,cap_setgid+ep /path/to/binary'")
	}
}

// HasSufficientPrivileges returns true if the process can change both UID and GID
func (r *PrivilegeVerificationResult) HasSufficientPrivileges() bool {
	return r.CanChangeUID && r.CanChangeGID
}

// HasPartialPrivileges returns true if the process can change at least one of UID or GID
func (r *PrivilegeVerificationResult) HasPartialPrivileges() bool {
	return r.CanChangeUID || r.CanChangeGID
}

// String returns a human-readable summary of the verification results
func (r *PrivilegeVerificationResult) String() string {
	status := "INSUFFICIENT"
	if r.HasSufficientPrivileges() {
		status = "SUFFICIENT"
	} else if r.HasPartialPrivileges() {
		status = "PARTIAL"
	}

	summary := fmt.Sprintf("Privilege Status: %s\n", status)
	summary += fmt.Sprintf("Current UID/GID: %d/%d, fsuid/fsgid: %d/%d\n",
		r.CurrentUID, r.CurrentGID, r.CurrentFsUID, r.CurrentFsGID)
	summary += fmt.Sprintf("Can change UID: %t, Can change GID: %t\n",
		r.CanChangeUID, r.CanChangeGID)

	if len(r.ErrorMessages) > 0 {
		summary += "Issues:\n"
		for _, msg := range r.ErrorMessages {
			summary += fmt.Sprintf("  - %s\n", msg)
		}
	}

	if len(r.Recommendations) > 0 {
		summary += "Recommendations:\n"
		for _, rec := range r.Recommendations {
			summary += fmt.Sprintf("  - %s\n", rec)
		}
	}

	return summary
}

// UserThreadPool manages a pool of threads, each dedicated to a specific user UID
type UserThreadPool struct {
	mu           sync.RWMutex
	threads      map[int]*UserThread // Maps UID to UserThread
	threadTTL    time.Duration       // Time to keep idle threads alive
	cleanupTimer *time.Timer
	nobodyUID    int // UID for nobody user (fallback)
	nobodyGID    int // GID for nobody group (fallback)
}

// UserThread represents a dedicated thread for a user with persistent UID
type UserThread struct {
	uid          int
	gid          int
	username     string
	threadID     int
	requestChan  chan *ThreadRequest
	responseChan chan *ThreadResponse
	quit         chan struct{}
	lastUsed     time.Time
	mu           sync.RWMutex
	active       bool
}

// ThreadRequest represents a request to execute on a user thread
type ThreadRequest struct {
	ID       string
	Function func() (interface{}, error)
	Context  context.Context
}

// ThreadResponse represents the response from a user thread
type ThreadResponse struct {
	ID     string
	Result interface{}
	Error  error
}

// UserThreadPoolConfig holds configuration for the thread pool
type UserThreadPoolConfig struct {
	ThreadTTL     time.Duration // How long to keep idle threads
	CleanupPeriod time.Duration // How often to check for expired threads
	NobodyUID     int           // UID for nobody user (fallback)
	NobodyGID     int           // GID for nobody group (fallback)
}

// NewUserThreadPool creates a new thread pool for managing per-user threads
func NewUserThreadPool(config UserThreadPoolConfig) (*UserThreadPool, *PrivilegeVerificationResult, error) {
	if config.ThreadTTL == 0 {
		config.ThreadTTL = 5 * time.Minute
	}
	if config.CleanupPeriod == 0 {
		config.CleanupPeriod = 1 * time.Minute
	}
	if config.NobodyUID == 0 {
		config.NobodyUID = 65534 // Default nobody UID
	}
	if config.NobodyGID == 0 {
		config.NobodyGID = 65534 // Default nobody GID
	}

	// Verify privileges before creating the thread pool
	privResult := VerifyPrivileges(config.NobodyUID, config.NobodyGID)

	pool := &UserThreadPool{
		threads:   make(map[int]*UserThread),
		threadTTL: config.ThreadTTL,
		nobodyUID: config.NobodyUID,
		nobodyGID: config.NobodyGID,
	}

	// Start cleanup routine
	pool.startCleanup(config.CleanupPeriod)

	return pool, privResult, nil
}

// GetOrCreateUserThread gets an existing user thread or creates a new one
func (p *UserThreadPool) GetOrCreateUserThread(ctx context.Context, user *userv1beta1.User) (*UserThread, error) {
	// Map user to UID/GID
	uid, gid := p.mapUserToUIDGID(user)

	p.mu.RLock()
	thread, exists := p.threads[uid]
	p.mu.RUnlock()

	if exists && thread.active {
		thread.updateLastUsed()
		return thread, nil
	}

	// Create new thread
	p.mu.Lock()
	defer p.mu.Unlock()

	// Double-check after acquiring write lock
	if thread, exists := p.threads[uid]; exists && thread.active {
		thread.updateLastUsed()
		return thread, nil
	}

	// Create new user thread
	thread = &UserThread{
		uid:          uid,
		gid:          gid,
		username:     user.Username,
		requestChan:  make(chan *ThreadRequest, 100),
		responseChan: make(chan *ThreadResponse, 100),
		quit:         make(chan struct{}),
		lastUsed:     time.Now(),
		active:       true,
	}

	p.threads[uid] = thread

	// Start the thread
	go thread.run()

	log := appctx.GetLogger(ctx)
	log.Debug().Int("uid", uid).Str("username", user.Username).Msg("Created new user thread")

	return thread, nil
}

// mapUserToUIDGID maps a user to UID/GID, with special handling for nobody user
func (p *UserThreadPool) mapUserToUIDGID(user *userv1beta1.User) (int, int) {
	// Handle nobody user specially
	if user.Id != nil && user.Id.OpaqueId == "nobody" {
		return p.nobodyUID, p.nobodyGID
	}

	// For other users, respect the UidNumber and GidNumber from the user struct if available
	// This allows tests to specify exact UIDs (e.g., root = 0)
	if user.UidNumber > 0 || user.GidNumber > 0 {
		uid := int(user.UidNumber)
		gid := int(user.GidNumber)
		
		// Allow UID 0 (root) explicitly
		if user.UidNumber == 0 {
			uid = 0
		}
		if user.GidNumber == 0 {
			gid = 0
		}
		
		return uid, gid
	}

	// For other users, you would implement proper UID/GID mapping here
	// This is a simplified example - in practice you'd:
	// 1. Query system user database (getpwnam)
	// 2. Use LDAP/AD mapping
	// 3. Use a configuration-based mapping
	// 4. Parse numeric UIDs from opaque ID if available

	// Default fallback for users without explicit UID/GID
	return 1000, 1000
}

// ExecuteOnUserThread executes a function on the appropriate user thread
func (p *UserThreadPool) ExecuteOnUserThread(ctx context.Context, user *userv1beta1.User, fn func() (interface{}, error)) (interface{}, error) {
	thread, err := p.GetOrCreateUserThread(ctx, user)
	if err != nil {
		return nil, err
	}

	return thread.Execute(ctx, fn)
}

// getExistingUserThread returns an existing user thread for the given UID, or nil if it doesn't exist
func (p *UserThreadPool) getExistingUserThread(uid int) (*UserThread, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	thread, exists := p.threads[uid]
	if !exists {
		return nil, nil
	}

	return thread, nil
}

// Shutdown gracefully shuts down all user threads
func (p *UserThreadPool) Shutdown() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.cleanupTimer != nil {
		p.cleanupTimer.Stop()
	}

	for uid, thread := range p.threads {
		thread.shutdown()
		delete(p.threads, uid)
	}
}

// startCleanup starts the cleanup routine for expired threads
func (p *UserThreadPool) startCleanup(period time.Duration) {
	p.cleanupTimer = time.AfterFunc(period, func() {
		p.cleanupExpiredThreads()
		p.startCleanup(period) // Schedule next cleanup
	})
}

// cleanupExpiredThreads removes threads that haven't been used recently
func (p *UserThreadPool) cleanupExpiredThreads() {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	for uid, thread := range p.threads {
		thread.mu.RLock()
		expired := now.Sub(thread.lastUsed) > p.threadTTL
		thread.mu.RUnlock()

		if expired {
			thread.shutdown()
			delete(p.threads, uid)
		}
	}
}

// run is the main loop for a user thread
func (ut *UserThread) run() {
	// Lock this goroutine to the current OS thread
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	// Get current thread ID
	ut.threadID = getTID()

	// Set the filesystem UID for this thread
	originalFsuid := setfsuidSafe(-1)
	originalFsgid := setfsgidSafe(-1)

	// Change to user's UID
	setfsuidSafe(ut.uid)
	setfsgidSafe(ut.gid)

	defer func() {
		// Restore original filesystem UID/GID when thread exits
		setfsuidSafe(originalFsuid)
		setfsgidSafe(originalFsgid)
		ut.active = false
	}()

	for {
		select {
		case request := <-ut.requestChan:
			// Execute the request on this thread with the correct UID
			result, err := request.Function()

			// Send response
			response := &ThreadResponse{
				ID:     request.ID,
				Result: result,
				Error:  err,
			}

			select {
			case ut.responseChan <- response:
			case <-ut.quit:
				return
			}

			ut.updateLastUsed()

		case <-ut.quit:
			return
		}
	}
}

// Execute executes a function on this user thread
func (ut *UserThread) Execute(ctx context.Context, fn func() (interface{}, error)) (interface{}, error) {
	if !ut.active {
		return nil, syscall.EINVAL
	}

	requestID := "req-" + time.Now().Format("20060102150405.000000")
	request := &ThreadRequest{
		ID:       requestID,
		Function: fn,
		Context:  ctx,
	}

	// Send request
	select {
	case ut.requestChan <- request:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// Wait for response
	select {
	case response := <-ut.responseChan:
		if response.ID == requestID {
			return response.Result, response.Error
		}
		// Wrong response ID - this shouldn't happen
		return nil, syscall.EIO

	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// updateLastUsed updates the last used timestamp
func (ut *UserThread) updateLastUsed() {
	ut.mu.Lock()
	ut.lastUsed = time.Now()
	ut.mu.Unlock()
}

// shutdown gracefully shuts down the user thread
func (ut *UserThread) shutdown() {
	ut.mu.Lock()
	defer ut.mu.Unlock()

	if ut.active {
		close(ut.quit)
		ut.active = false
	}
}

// GetStats returns statistics about the thread
func (ut *UserThread) GetStats() map[string]interface{} {
	ut.mu.RLock()
	defer ut.mu.RUnlock()

	return map[string]interface{}{
		"uid":        ut.uid,
		"gid":        ut.gid,
		"username":   ut.username,
		"thread_id":  ut.threadID,
		"last_used":  ut.lastUsed,
		"active":     ut.active,
		"queue_size": len(ut.requestChan),
	}
}

// getTID gets the Linux thread ID
func getTID() int {
	const SYS_GETTID = 186
	tid, _, _ := syscall.Syscall(SYS_GETTID, 0, 0, 0)
	return int(tid)
}

// setfsuidSafe wraps the setfsuid syscall with error handling
func setfsuidSafe(uid int) int {
	const SYS_SETFSUID = 122
	prevUID, _, _ := syscall.Syscall(SYS_SETFSUID, uintptr(uid), 0, 0)
	return int(prevUID)
}

// setfsgidSafe wraps the setfsgid syscall with error handling
func setfsgidSafe(gid int) int {
	const SYS_SETFSGID = 123
	prevGID, _, _ := syscall.Syscall(SYS_SETFSGID, uintptr(gid), 0, 0)
	return int(prevGID)
}
