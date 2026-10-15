package docker

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/moby/moby/client"
)

const MinimumAPIVersion = "1.41"

var apiVersionPattern = regexp.MustCompile(`^1\.[0-9]+$`)

// NewClient negotiates the API range used by this exporter. The SDK's generic
// negotiator rejects API <1.44, although our read-only endpoints work on 1.41+.
// Never pretend that the daemon supports a higher version or rewrite URLs.
func NewClient(ctx context.Context, host string) (*client.Client, error) {
	probe, err := client.New(client.WithHost(host))
	if err != nil {
		return nil, err
	}
	ping, err := probe.Ping(ctx, client.PingOptions{})
	probe.Close()
	if err != nil {
		return nil, fmt.Errorf("Docker ping: %w", err)
	}
	version := ping.APIVersion
	if !apiVersionPattern.MatchString(version) {
		return nil, fmt.Errorf("Docker returned missing or invalid API version %q", version)
	}
	minor, err := strconv.Atoi(strings.TrimPrefix(version, "1."))
	if err != nil || minor < 41 {
		return nil, fmt.Errorf("Docker API %q is unsupported; minimum supported API is %s", version, MinimumAPIVersion)
	}
	maxMinor, _ := strconv.Atoi(strings.TrimPrefix(client.MaxAPIVersion, "1."))
	if minor > maxMinor {
		version = client.MaxAPIVersion
	}
	return client.New(client.WithHost(host), client.WithVersion(version))
}
