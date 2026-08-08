// Package plugins is the shared subprocess plugin protocol: JSON-over-
// stdin/stdout process execution (Call) and executable discovery
// (ListExecutables), used by both the Provider and Target plugin adapters
// (pkg/managers/providers, pkg/managers/installer). It knows nothing about
// Provider or Target domain types — imports no pkg/managers/* package
// (003-engineering-optimization R2/FR-019).
package plugins
