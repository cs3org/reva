package static

import (
	"testing"

	ua "github.com/mileusna/useragent"
)

func TestUserAgentIsAllowed(t *testing.T) {

	tests := []struct {
		description string
		userAgent   string
		userAgents  []string
		expected    bool
	}{
		{
			description: "grpc-go",
			userAgent:   "grpc-go",
			userAgents:  []string{"grpc"},
			expected:    true,
		},
		{
			description: "grpc-go",
			userAgent:   "grpc-go",
			userAgents:  []string{"desktop", "mobile", "web"},
			expected:    false,
		},
		{
			description: "web-firefox",
			userAgent:   "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/15.0 Safari/605.1.15",
			userAgents:  []string{"web"},
			expected:    true,
		},
		{
			description: "web-firefox",
			userAgent:   "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/15.0 Safari/605.1.15",
			userAgents:  []string{"desktop", "mobile", "grpc"},
			expected:    false,
		},
		{
			description: "desktop-mirall",
			userAgent:   "Mozilla/5.0 (Linux) mirall/2.7.1 (build 2596) (cernboxcmd, centos-3.10.0-1160.36.2.el7.x86_64 ClientArchitecture: x86_64 OsArchitecture: x86_64)",
			userAgents:  []string{"desktop"},
			expected:    true,
		},
		{
			description: "desktop-mirall",
			userAgent:   "Mozilla/5.0 (Linux) mirall/2.7.1 (build 2596) (cernboxcmd, centos-3.10.0-1160.36.2.el7.x86_64 ClientArchitecture: x86_64 OsArchitecture: x86_64)",
			userAgents:  []string{"web", "mobile", "grpc"},
			expected:    false,
		},
		{
			description: "mobile-android",
			userAgent:   "Mozilla/5.0 (Android) ownCloud-android/2.13.1 cernbox/Android",
			userAgents:  []string{"mobile"},
			expected:    true,
		},
		{
			description: "mobile-ios",
			userAgent:   "Mozilla/5.0 (iOS) ownCloud-iOS/3.8.0 cernbox/iOS",
			userAgents:  []string{"mobile"},
			expected:    true,
		},
		{
			description: "mobile-android",
			userAgent:   "Mozilla/5.0 (Android) ownCloud-android/2.13.1 cernbox/Android",
			userAgents:  []string{"web", "desktop", "grpc"},
			expected:    false,
		},
		{
			description: "mobile-ios",
			userAgent:   "Mozilla/5.0 (iOS) ownCloud-iOS/3.8.0 cernbox/iOS",
			userAgents:  []string{"web", "desktop", "grpc"},
			expected:    false,
		},
		{
			description: "mobile-web",
			userAgent:   "Mozilla/5.0 (Android 11; Mobile; rv:86.0) Gecko/86.0 Firefox/86.0",
			userAgents:  []string{"web"},
			expected:    true,
		},
		{
			description: "mobile-web",
			userAgent:   "Mozilla/5.0 (Android 11; Mobile; rv:86.0) Gecko/86.0 Firefox/86.0",
			userAgents:  []string{"desktop", "grpc", "mobile"},
			expected:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {

			ua := ua.Parse(tt.userAgent)

			res := userAgentIsAllowed(&ua, tt.userAgents)

			if res != tt.expected {
				t.Fatalf("result does not match with expected. got=%+v expected=%+v", res, tt.expected)
			}

		})
	}
}
