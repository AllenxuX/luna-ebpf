package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPrecedence(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.yaml")
	os.WriteFile(p, []byte("listen_address: '127.0.0.1:9999'\nmetrics_poll_interval: 2s\n"), 0644)
	c, err := Parse([]string{"--config", p, "--listen-address", "127.0.0.1:1234"})
	if err != nil {
		t.Fatal(err)
	}
	if c.Listen != "127.0.0.1:1234" || c.Poll != 2*time.Second || c.Reconcile != 30*time.Second {
		t.Fatalf("%+v", c)
	}
	for _, a := range [][]string{{"--metrics-poll-interval", "0s"}, {"--map-max-entries", "1"}, {"--log-level", "bogus"}, {"--unknown"}} {
		if _, err = Parse(a); err == nil {
			t.Fatalf("accepted %v", a)
		}
	}
	os.WriteFile(p, []byte("typo: true\n"), 0644)
	if _, err = Parse([]string{"--config", p}); err == nil {
		t.Fatal("unknown yaml")
	}
}
