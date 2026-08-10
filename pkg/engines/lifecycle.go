package engines

// LifecycleOptions controls archive/unarchive/delete/convert.
type LifecycleOptions struct {
	Force  bool
	DryRun bool
}
