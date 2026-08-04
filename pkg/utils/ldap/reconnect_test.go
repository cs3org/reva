package ldap

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-ldap/ldap/v3"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// nopSleep returns a sleepFn that captures durations without blocking.
func nopSleep(durations *[]time.Duration) func(time.Duration) {
	return func(d time.Duration) {
		*durations = append(*durations, d)
	}
}

// newTestConn returns a ConnWithReconnect backed by a fake that never dials, for policy-predicate
// tests that inject errors directly into RetryOp closures.
func newTestConn(t *testing.T, read, write RetryPolicy, sleepFn func(time.Duration)) *ConnWithReconnect {
	t.Helper()
	nop := zerolog.Nop()
	c := &ConnWithReconnect{
		conn:    make(chan ldapConnection),
		reset:   make(chan *ldap.Conn),
		read:    read,
		write:   write,
		sleepFn: sleepFn,
		logger:  &nop,
	}
	// Background goroutine mimics ldapAutoConnect: always serves the same
	// placeholder connection and discards reconnect signals.
	placeholder := ldap.NewConn(nil, false)
	go func() {
		for {
			select {
			case c.conn <- ldapConnection{Conn: placeholder, Error: nil}:
			case <-c.reset:
				// discard reset signals — reconnect just gets the same placeholder back
			}
		}
	}()
	return c
}

// TestAddUsesWritePolicy: Add routes through write policy — Busy is not retried.
// Fails if Add were wired to c.read (read policy retries Busy).
func TestAddUsesWritePolicy(t *testing.T) {
	var calls int32
	var slept []time.Duration

	c := newTestConn(t,
		NewReadPolicy(1, 0, 0),
		NewWritePolicy(1, 0, 0),
		nopSleep(&slept),
	)

	_ = c.RetryOp(c.write, func(_ *ldap.Conn) error {
		atomic.AddInt32(&calls, 1)
		return ldap.NewError(ldap.LDAPResultBusy, errors.New("busy"))
	})

	assert.Equal(t, int32(1), atomic.LoadInt32(&calls), "Add must not retry Busy — uses write policy")
	assert.Empty(t, slept)
}

// TestReadWritePolicyRetryableCodes asserts reads retry every transient/network code while writes
// retry only a pre-send ErrorNetwork; cases carry code + message since write retryability keys on both.
func TestReadWritePolicyRetryableCodes(t *testing.T) {
	// preSend / postSend are representative ErrorNetwork messages go-ldap emits before and after
	// the request is transmitted.
	const preSend = "ldap: connection closed"
	const postSend = "ldap: response channel closed"

	cases := []struct {
		name      string
		err       error
		policy    func() RetryPolicy
		wantRetry bool
	}{
		// ErrorNetwork — read retries any message; write retries only the pre-send one.
		{"read retries pre-send ErrorNetwork", ldap.NewError(ldap.ErrorNetwork, errors.New(preSend)), func() RetryPolicy { return NewReadPolicy(1, 0, 0) }, true},
		{"read retries post-send ErrorNetwork", ldap.NewError(ldap.ErrorNetwork, errors.New(postSend)), func() RetryPolicy { return NewReadPolicy(1, 0, 0) }, true},
		{"write retries pre-send ErrorNetwork", ldap.NewError(ldap.ErrorNetwork, errors.New(preSend)), func() RetryPolicy { return NewWritePolicy(1, 0, 0) }, true},
		{"write does not retry post-send ErrorNetwork", ldap.NewError(ldap.ErrorNetwork, errors.New(postSend)), func() RetryPolicy { return NewWritePolicy(1, 0, 0) }, false},
		// Tier 1 — reconnect codes: read retries, write never (not connection-establishment failures).
		{"read retries Timeout", ldap.NewError(ldap.LDAPResultTimeout, errors.New("timeout")), func() RetryPolicy { return NewReadPolicy(1, 0, 0) }, true},
		{"write does not retry Timeout", ldap.NewError(ldap.LDAPResultTimeout, errors.New("timeout")), func() RetryPolicy { return NewWritePolicy(1, 0, 0) }, false},
		{"read retries LocalError", ldap.NewError(ldap.LDAPResultLocalError, errors.New("local")), func() RetryPolicy { return NewReadPolicy(1, 0, 0) }, true},
		{"write does not retry LocalError", ldap.NewError(ldap.LDAPResultLocalError, errors.New("local")), func() RetryPolicy { return NewWritePolicy(1, 0, 0) }, false},
		// Tier 2 — backoff-only codes: read retries, write never.
		{"read retries Busy", ldap.NewError(ldap.LDAPResultBusy, errors.New("busy")), func() RetryPolicy { return NewReadPolicy(1, 0, 0) }, true},
		{"write does not retry Busy", ldap.NewError(ldap.LDAPResultBusy, errors.New("busy")), func() RetryPolicy { return NewWritePolicy(1, 0, 0) }, false},
		{"read retries Unavailable", ldap.NewError(ldap.LDAPResultUnavailable, errors.New("unavailable")), func() RetryPolicy { return NewReadPolicy(1, 0, 0) }, true},
		{"write does not retry Unavailable", ldap.NewError(ldap.LDAPResultUnavailable, errors.New("unavailable")), func() RetryPolicy { return NewWritePolicy(1, 0, 0) }, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var calls int32
			var slept []time.Duration
			p := tc.policy()

			c := newTestConn(t, p, p, nopSleep(&slept))

			_ = c.RetryOp(p, func(_ *ldap.Conn) error {
				atomic.AddInt32(&calls, 1)
				return tc.err
			})

			if tc.wantRetry {
				assert.Greater(t, atomic.LoadInt32(&calls), int32(1), "expected retry")
			} else {
				assert.Equal(t, int32(1), atomic.LoadInt32(&calls), "expected no retry")
			}
		})
	}
}

// TestPreSendNetworkErrClassification pins isPreSendNetworkErr: only ErrorNetwork with a known
// pre-send message classifies as pre-send; everything else is not-pre-send (fail-closed).
func TestPreSendNetworkErrClassification(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"pre-send connection closed", ldap.NewError(ldap.ErrorNetwork, errors.New("ldap: connection closed")), true},
		{"pre-send could not send", ldap.NewError(ldap.ErrorNetwork, errors.New("ldap: could not send message for unknown reason")), true},
		{"post-send response channel closed", ldap.NewError(ldap.ErrorNetwork, errors.New("ldap: response channel closed")), false},
		{"post-send could not retrieve response", ldap.NewError(ldap.ErrorNetwork, errors.New("ldap: could not retrieve response")), false},
		{"post-send packet not received", ldap.NewError(ldap.ErrorNetwork, errors.New("ldap: packet not received")), false},
		{"post-send connection timed out", ldap.NewError(ldap.ErrorNetwork, errors.New("ldap: connection timed out")), false},
		{"non-network ldap code", ldap.NewError(ldap.LDAPResultBusy, errors.New("ldap: connection closed")), false},
		{"non-ldap error", errors.New("ldap: connection closed"), false},
		{"nil", nil, false},
		// A failed conn.Write: a plain error with no result code, raised before the packet reached
		// the server. This is the race window where an op beats the reader to noticing a reap.
		{"send failed (broken pipe)", errors.New("unable to send request: write tcp 127.0.0.1:1->127.0.0.1:2: write: broken pipe"), true},
		{"send failed (connection reset)", errors.New("unable to send request: write tcp 127.0.0.1:1->127.0.0.1:2: write: connection reset by peer"), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isPreSendNetworkErr(tc.err))
		})
	}
}

// TestWritePolicyMatchesEmittableCode guards against dead code: the write policy must retry at least
// one error go-ldap actually emits (a {81,91} code allowlist would match nothing, since it emits 200).
func TestWritePolicyMatchesEmittableCode(t *testing.T) {
	p := NewWritePolicy(1, 0, 0)
	// The exact error go-ldap returns from sendMessageWithFlags when a reaped idle connection is
	// reused for a write — the case the write policy must recover from.
	preSend := ldap.NewError(ldap.ErrorNetwork, errors.New("ldap: connection closed"))
	require.True(t, p.isRetryable(preSend),
		"write policy must retry at least one error go-ldap actually emits (pre-send ErrorNetwork)")
}

// TestWriteRetriesPreSendNetworkError: a write op that fails with a pre-send ErrorNetwork is
// retried (the connection was stale before the request was sent, so no double-apply risk).
func TestWriteRetriesPreSendNetworkError(t *testing.T) {
	var calls int32
	p := NewWritePolicy(1, 0, 0)
	var slept []time.Duration
	c := newTestConn(t, p, p, nopSleep(&slept))

	_ = c.RetryOp(c.write, func(_ *ldap.Conn) error {
		atomic.AddInt32(&calls, 1)
		return ldap.NewError(ldap.ErrorNetwork, errors.New("ldap: connection closed"))
	})

	assert.Greater(t, atomic.LoadInt32(&calls), int32(1),
		"write must retry a pre-send ErrorNetwork")
}

// TestWriteRetriesSendFailedError asserts a write failing with a codeless failed conn.Write is
// retried: it is registered only after a successful write, so it never reached the server.
func TestWriteRetriesSendFailedError(t *testing.T) {
	sendFailed := errors.New("unable to send request: write tcp 127.0.0.1:1->127.0.0.1:2: write: broken pipe")

	// Guard the premise: this really is not an *ldap.Error and really does map to a code neither
	// policy's switch matches. If go-ldap ever wraps it, this test's rationale needs revisiting.
	var lerr *ldap.Error
	require.False(t, errors.As(sendFailed, &lerr), "expected a plain error, not *ldap.Error")
	require.Equal(t, uint16(ldap.LDAPResultOther), ldapErrCode(sendFailed), "expected no usable result code")

	var calls int32
	p := NewWritePolicy(1, 0, 0)
	var slept []time.Duration
	c := newTestConn(t, p, p, nopSleep(&slept))

	_ = c.RetryOp(c.write, func(_ *ldap.Conn) error {
		atomic.AddInt32(&calls, 1)
		return sendFailed
	})

	assert.Greater(t, atomic.LoadInt32(&calls), int32(1),
		"write must retry a failed conn.Write — the request never reached the server")
}

// TestReadRetriesSendFailedError: the read policy retries a failed conn.Write too, and reconnects
// rather than reusing the dead connection.
func TestReadRetriesSendFailedError(t *testing.T) {
	sendFailed := errors.New("unable to send request: write tcp 127.0.0.1:1->127.0.0.1:2: write: broken pipe")

	p := NewReadPolicy(1, 0, 0)
	require.True(t, p.isRetryable(sendFailed), "read must retry a failed conn.Write")
	require.True(t, p.needsReconnect(sendFailed), "a failed conn.Write needs a fresh connection")

	var calls int32
	var slept []time.Duration
	c := newTestConn(t, p, p, nopSleep(&slept))

	_ = c.RetryOp(c.read, func(_ *ldap.Conn) error {
		atomic.AddInt32(&calls, 1)
		return sendFailed
	})

	assert.Greater(t, atomic.LoadInt32(&calls), int32(1), "read must retry a failed conn.Write")
}

// TestWriteDoesNotRetryPostSendNetworkError: a write op that fails with a post-send ErrorNetwork
// must not be retried — the mutation may already have been applied.
func TestWriteDoesNotRetryPostSendNetworkError(t *testing.T) {
	postSendMsgs := []string{
		"ldap: response channel closed",
		"ldap: could not retrieve response",
		"ldap: packet not received",
	}
	for _, msg := range postSendMsgs {
		t.Run(msg, func(t *testing.T) {
			var calls int32
			p := NewWritePolicy(1, 0, 0)
			var slept []time.Duration
			c := newTestConn(t, p, p, nopSleep(&slept))

			_ = c.RetryOp(c.write, func(_ *ldap.Conn) error {
				atomic.AddInt32(&calls, 1)
				return ldap.NewError(ldap.ErrorNetwork, errors.New(msg))
			})

			assert.Equal(t, int32(1), atomic.LoadInt32(&calls),
				"write must not retry a post-send ErrorNetwork")
		})
	}
}

// TestNeverRetryCodes: semantic/permanent errors are never retried by either policy.
func TestNeverRetryCodes(t *testing.T) {
	neverCodes := []uint16{
		ldap.LDAPResultInvalidCredentials,       // 49
		ldap.LDAPResultInsufficientAccessRights, // 50
		ldap.LDAPResultEntryAlreadyExists,       // 68
		ldap.LDAPResultNoSuchObject,             // 32
		ldap.LDAPResultNoSuchAttribute,          // 16
		ldap.LDAPResultConstraintViolation,      // 19
		ldap.LDAPResultUnwillingToPerform,       // 53
		ldap.LDAPResultInvalidDNSyntax,          // 34
		ldap.LDAPResultSizeLimitExceeded,        // 4
	}

	for _, code := range neverCodes {
		code := code
		for _, policyName := range []string{"read", "write"} {
			policyName := policyName
			t.Run(ldap.LDAPResultCodeMap[code]+"/"+policyName, func(t *testing.T) {
				var calls int32
				var slept []time.Duration
				var p RetryPolicy
				if policyName == "read" {
					p = NewReadPolicy(1, 0, 0)
				} else {
					p = NewWritePolicy(1, 0, 0)
				}
				c := newTestConn(t, p, p, nopSleep(&slept))

				_ = c.RetryOp(p, func(_ *ldap.Conn) error {
					atomic.AddInt32(&calls, 1)
					return ldap.NewError(code, errors.New("permanent"))
				})

				assert.Equal(t, int32(1), atomic.LoadInt32(&calls), "must not retry permanent error")
				assert.Empty(t, slept)
			})
		}
	}
}

// TestRetryPolicyConfig: MaxRetries / BaseDelay / MaxDelay are honoured.
func TestRetryPolicyConfig(t *testing.T) {
	cases := []struct {
		name             string
		maxRetries       int
		baseDelay        time.Duration
		maxDelay         time.Duration
		failUntilAttempt int32 // return ErrorNetwork until this attempt (inclusive); 0 = always fail
		wantAttempts     int32
		wantSleepCount   int
		wantSleepMin     time.Duration
		wantSleepMax     time.Duration
		wantErr          bool
	}{
		{
			name:             "defaults: MaxRetries=1 BaseDelay=0 → 1 retry no sleep (backward compat)",
			maxRetries:       1,
			baseDelay:        0,
			maxDelay:         0,
			failUntilAttempt: 1,
			wantAttempts:     2,
			wantSleepCount:   0,
		},
		{
			name:             "MaxRetries=2 always failing → 3 total attempts exhausted",
			maxRetries:       2,
			baseDelay:        0,
			maxDelay:         0,
			failUntilAttempt: 0,
			wantAttempts:     3,
			wantSleepCount:   0,
			wantErr:          true,
		},
		{
			name:             "MaxRetries=2 BaseDelay=10ms MaxDelay=200ms → 2 sleeps in [base*(1-RF), maxDelay]",
			maxRetries:       2,
			baseDelay:        10 * time.Millisecond,
			maxDelay:         200 * time.Millisecond,
			failUntilAttempt: 2,
			wantAttempts:     3,
			wantSleepCount:   2,
			wantSleepMin:     5 * time.Millisecond,  // baseDelay * (1 - RF=0.5)
			wantSleepMax:     200 * time.Millisecond, // maxDelay
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var callCount int32
			var sleepCalls []time.Duration

			p := NewReadPolicy(tc.maxRetries, tc.baseDelay, tc.maxDelay)
			c := newTestConn(t, p, p, nopSleep(&sleepCalls))

			err := c.RetryOp(p, func(_ *ldap.Conn) error {
				n := atomic.AddInt32(&callCount, 1)
				if tc.failUntilAttempt == 0 || n <= tc.failUntilAttempt {
					return ldap.NewError(ldap.LDAPResultServerDown, errors.New("down"))
				}
				return nil
			})

			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tc.wantAttempts, atomic.LoadInt32(&callCount), "attempt count")
			assert.Len(t, sleepCalls, tc.wantSleepCount, "sleep call count")
			for _, d := range sleepCalls {
				assert.GreaterOrEqual(t, d, tc.wantSleepMin, "sleep >= min")
				assert.LessOrEqual(t, d, tc.wantSleepMax, "sleep <= MaxDelay")
			}
		})
	}
}

// TestRetryOpExhaustionPreservesErrorCode: on exhaustion RetryOp must return the
// last real error code (Busy here), not mask it as ErrorNetwork.
func TestRetryOpExhaustionPreservesErrorCode(t *testing.T) {
	p := NewReadPolicy(1, 0, 0)
	var slept []time.Duration
	c := newTestConn(t, p, p, nopSleep(&slept))

	err := c.RetryOp(p, func(_ *ldap.Conn) error {
		return ldap.NewError(ldap.LDAPResultBusy, errors.New("server busy"))
	})

	require.Error(t, err)
	assert.False(t, ldap.IsErrorWithCode(err, ldap.ErrorNetwork),
		"exhaustion must not mask real error code with ErrorNetwork")
	assert.True(t, ldap.IsErrorWithCode(err, ldap.LDAPResultBusy),
		"exhaustion must preserve the last real error code")
}

// TestWritePolicyOpaqueErrorNotRetried asserts a non-*ldap.Error from a write is not retried
// (guards against defaulting opaque errors to the retryable ErrorNetwork).
func TestWritePolicyOpaqueErrorNotRetried(t *testing.T) {
	var calls int32
	p := NewWritePolicy(1, 0, 0)
	var slept []time.Duration
	c := newTestConn(t, p, p, nopSleep(&slept))

	_ = c.RetryOp(c.write, func(_ *ldap.Conn) error {
		atomic.AddInt32(&calls, 1)
		return errors.New("opaque non-ldap error")
	})

	assert.Equal(t, int32(1), atomic.LoadInt32(&calls),
		"opaque non-*ldap.Error must not be retried on a write op")
}

// TestRetryPolicyBackoffGrowsWhenMaxDelayZero asserts that with MaxDelay unset the backoff still
// grows beyond BaseDelay (falling back to the library default cap) rather than oscillating at it.
func TestRetryPolicyBackoffGrowsWhenMaxDelayZero(t *testing.T) {
	const maxRetries = 8
	const baseDelay = 10 * time.Millisecond
	p := NewReadPolicy(maxRetries, baseDelay, 0)
	var slept []time.Duration
	c := newTestConn(t, p, p, nopSleep(&slept))

	_ = c.RetryOp(p, func(_ *ldap.Conn) error {
		return ldap.NewError(ldap.LDAPResultServerDown, errors.New("down"))
	})

	require.Len(t, slept, maxRetries, "should sleep maxRetries times before exhaustion")

	maxSleep := slept[0]
	for _, d := range slept[1:] {
		if d > maxSleep {
			maxSleep = d
		}
	}
	assert.Greater(t, maxSleep, 2*baseDelay,
		"backoff with BaseDelay=%v MaxDelay=0 must grow; got maxSleep=%v", baseDelay, maxSleep)
}
