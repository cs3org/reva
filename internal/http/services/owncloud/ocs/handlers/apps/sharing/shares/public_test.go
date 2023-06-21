package shares

import (
	"testing"
)

func TestHandler_enforcePassword(t *testing.T) {
	tests := []struct {
		name    string
		h       *Handler
		permKey int
		exp     bool
	}{
		{
			name: "enforce permission 1",
			h: &Handler{
				publicPasswordEnforced: passwordEnforced{
					EnforcedForReadOnly: true,
				},
			},
			permKey: 1,
			exp:     true,
		},
		{
			name: "enforce permission 3",
			h: &Handler{
				publicPasswordEnforced: passwordEnforced{
					EnforcedForReadWrite: true,
				},
			},
			permKey: 3,
			exp:     true,
		},
		{
			name: "enforce permission 4",
			h: &Handler{
				publicPasswordEnforced: passwordEnforced{
					EnforcedForUploadOnly: true,
				},
			},
			permKey: 4,
			exp:     true,
		},
		{
			name: "enforce permission 5",
			h: &Handler{
				publicPasswordEnforced: passwordEnforced{
					EnforcedForReadWrite: true,
				},
			},
			permKey: 5,
			exp:     true,
		},
		{
			name: "enforce permission 15",
			h: &Handler{
				publicPasswordEnforced: passwordEnforced{
					EnforcedForReadWriteDelete: true,
				},
			},
			permKey: 15,
			exp:     true,
		},
		{
			name: "enforce permission 3 failed",
			h: &Handler{
				publicPasswordEnforced: passwordEnforced{
					EnforcedForReadOnly: true,
				},
			},
			permKey: 3,
			exp:     false,
		},
		{
			name: "enforce permission 8 failed",
			h: &Handler{
				publicPasswordEnforced: passwordEnforced{
					EnforcedForReadWriteDelete: true,
					EnforcedForUploadOnly:      true,
				},
			},
			permKey: 8,
			exp:     false,
		},
		{
			name: "enforce permission 11 failed",
			h: &Handler{
				publicPasswordEnforced: passwordEnforced{
					EnforcedForReadWriteDelete: true,
				},
			},
			permKey: 11,
			exp:     false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.h.enforcePassword(&tt.permKey) != tt.exp {
				t.Errorf("enforcePassword(\"%v\") returned %v instead of expected %v", tt.permKey, tt.h.enforcePassword(&tt.permKey), tt.exp)
			}
		})
	}
}
