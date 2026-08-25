// Package providers declares and implements the acquisition sources skm ships
// with — where an entry can be fetched from. It is the counterpart of
// pkg/targets (where an entry can be installed to); unlike a target, a
// built-in provider is code, so the implementations live here beside the
// Provider interface they satisfy.
//
// The Registry holds built-in and plugin providers in resolution order;
// subprocess plugin transport itself lives in pkg/plugins.
package providers
