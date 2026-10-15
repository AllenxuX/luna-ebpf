package agent

import "testing"

func TestV1RootMountValidation(t *testing.T) {
	full := []byte("42 30 0:28 / /sys/fs/cgroup/memory rw - cgroup cgroup rw,memory\n")
	if err := ValidateMountVersion(full, "/sys/fs/cgroup/memory", "v1"); err != nil {
		t.Fatal(err)
	}
	for _, root := range []string{"/sys/fs/cgroup", "/sys/fs/cgroup/memory/docker"} {
		if ValidateMountVersion(full, root, "v1") == nil {
			t.Fatal("accepted non-mount root")
		}
	}
	if ValidateMountVersion([]byte("42 30 0:28 /docker /sys/fs/cgroup/memory rw - cgroup cgroup rw,memory\n"), "/sys/fs/cgroup/memory", "v1") == nil {
		t.Fatal("accepted subtree bind")
	}
	if ValidateMountVersion(full, "/sys/fs/cgroup/memory", "v2") == nil {
		t.Fatal("accepted wrong filesystem")
	}
}

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
