package services

import (
	"context"
	"fmt"

	"github.com/alswl/skm/skm/pkg/common"
)

// PluginKind separates acquisition plugins from installation plugins while
// keeping their lifecycle, diagnostics and request framing identical.
type PluginKind string

const (
	PluginKindProvider PluginKind = "provider"
	PluginKindTarget   PluginKind = "target"
)

// PluginDescriptor is the common identity and capability envelope returned by
// every plugin. Built-ins are in-process plugins; external executables are
// subprocess plugins. Consumers must not need to special-case either form.
type PluginDescriptor struct {
	Version     int
	Kind        PluginKind
	ID          string
	Label       string
	Description string
	Path        string
}

// PluginRequest and PluginResponse are the in-process representation of the
// versioned plugin protocol. The external v1 JSON protocols remain accepted
// at the boundary and are translated by their adapters.
type PluginRequest struct {
	Version int
	Action  string
	Address string
	Entry   *common.Entry
	Target  *common.InstallTarget
	Force   bool
}

type PluginResponse struct {
	Descriptor PluginDescriptor
	Capability Capability
	Address    string
	Path       string
	Result     *bool
	State      common.InstallState
	Diff       string
	Dangling   []DanglingInstall
}

// Plugin is the shared host contract. It deliberately standardizes transport
// and lifecycle only: provider and target actions retain typed domain
// adapters instead of being flattened into an untyped mega-interface.
type Plugin interface {
	Descriptor() PluginDescriptor
	Handle(context.Context, PluginRequest) (PluginResponse, error)
}

// pluginProtocolVersion is the Target/Provider plugin protocol version skm
// itself implements. Plugins declare the version in their `id` response
// ("protocol_version"), defaulting to v1 when absent so older plugins keep
// loading; a trailing version gets a startup warning and v2-only actions
// (remove_foreign) a clear error.
const pluginProtocolVersion = 2

// removeForeignProtocolVersion is the minimum Target plugin protocol version
// that implements the remove_foreign action (conflict cleanup).
const removeForeignProtocolVersion = 2

func unsupportedPluginAction(d PluginDescriptor, action string) error {
	return fmt.Errorf("%s plugin %q does not support action %q", d.Kind, d.ID, action)
}
