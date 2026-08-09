// Package tui implements the Bubble Tea user interface. It is internal and
// must not be imported from outside this module.
//
// The package keeps one unexported Bubble Tea model so screen transitions and
// callbacks share a single state owner. Files are arranged by responsibility:
// app.go owns runtime/model/event-loop plumbing; catalog_state.go owns list
// filtering and navigation state; catalog_page.go owns catalog rendering;
// detail_state.go owns filesystem-backed detail preparation; action_*.go own
// user-initiated service calls; and components/ plus widgets/ contain stateless
// rendering primitives and modal values.
package tui
