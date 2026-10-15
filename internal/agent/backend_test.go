package agent

import "testing"

func TestSelectBackend(t *testing.T) {
	for _, tc := range []struct {
		version, request, want string
		invalid                bool
	}{
		{"1", "auto", "socket-trace", false},
		{"2", "auto", "cgroup-skb", false},
		{"1", "socket-trace", "socket-trace", false},
		{"2", "socket-trace", "socket-trace", false},
		{"2", "cgroup-skb", "cgroup-skb", false},
		{"1", "cgroup-skb", "", true},
		{"", "auto", "", true},
		{"3", "auto", "", true},
		{"2", "invalid", "", true},
	} {
		t.Run(tc.version+"/"+tc.request, func(t *testing.T) {
			got, err := selectBackend(tc.version, tc.request)
			if (err != nil) != tc.invalid || got != tc.want {
				t.Fatalf("got %q, %v; want %q invalid=%v", got, err, tc.want, tc.invalid)
			}
		})
	}
}
