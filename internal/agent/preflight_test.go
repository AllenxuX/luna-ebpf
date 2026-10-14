package agent

import "testing"

func TestRootMountValidation(t *testing.T) {
	full := "42 30 0:28 / /sys/fs/cgroup rw - cgroup2 cgroup rw\n"
	if err := ValidateMount([]byte(full), "/sys/fs/cgroup"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateMount([]byte(full), "/sys/fs/cgroup/docker"); err == nil {
		t.Fatal("subtree accepted as root")
	}
	sub := "42 30 0:28 /docker /sys/fs/cgroup rw - cgroup2 cgroup rw\n"
	if err := ValidateMount([]byte(sub), "/sys/fs/cgroup"); err == nil {
		t.Fatal("subtree bind accepted")
	}
}
