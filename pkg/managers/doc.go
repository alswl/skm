// Package managers orchestrates CLI and TUI use cases (scenarios) over the
// single-model pkg/services/* packages and pkg/dal. It is the single write
// path for all mutations, and the larger-granularity layer in
// 003-engineering-optimization's manager/service split: a manager may call
// multiple services and/or dal directly; each service owns one model.
package managers
