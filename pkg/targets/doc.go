// Package targets declares the install targets skm ships with — where an entry
// can be installed to. It is the counterpart of pkg/providers (where an entry
// can be acquired from), and holds declarations only: a built-in target is a
// record, not code. How anything is actually installed is pkg/installer's job,
// dispatched on common.InstallStrategy, so adding a built-in here adds no
// installation code anywhere.
//
// pkg/config consumes Builtins for the defaults a user gets with no
// config.yaml, for merging, and for the path-divergence report in
// `target list`.
package targets
