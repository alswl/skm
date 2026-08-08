// Package plugins is the shared subprocess plugin protocol: JSON-over-
// stdin/stdout process execution (Call) and executable discovery
// (ListExecutables), used by both the Provider and Target plugin adapters
// (pkg/services). It knows nothing about Provider or Target domain types —
// imports no services package
// (003-engineering-optimization R2/FR-019).
package plugins
