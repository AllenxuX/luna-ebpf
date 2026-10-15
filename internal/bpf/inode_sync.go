package bpf

// syncInodeOwners frees stale slots before inserting replacements, so an
// inventory that fits does not temporarily exceed the map capacity.
func syncInodeOwners(existing, desired map[uint64]uint64, remove func(uint64) error, update func(uint64, uint64) error) error {
	for inode, owner := range existing {
		if desired[inode] != owner {
			if err := remove(inode); err != nil {
				return err
			}
		}
	}
	for inode, owner := range desired {
		if existing[inode] == owner {
			continue
		}
		if err := update(inode, owner); err != nil {
			return err
		}
	}
	return nil
}
