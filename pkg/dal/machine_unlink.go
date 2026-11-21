package dal

// MachineLinkIdentity records the filesystem identity and raw target of a link.
type MachineLinkIdentity struct {
	Device uint64 `json:"device"`
	Inode  uint64 `json:"inode"`
	Target string `json:"target"`
}
