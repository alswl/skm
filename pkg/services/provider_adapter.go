package services

import "context"

// protocolProvider makes every existing built-in Provider an in-process
// Plugin. It is intentionally an adapter so third-party Go callers that
// implement the historical Provider interface keep working during migration.
type protocolProvider struct{ legacy Provider }

func asProtocolProvider(p Provider) Provider {
	if _, ok := p.(Plugin); ok {
		return p
	}
	return protocolProvider{legacy: p}
}

func (p protocolProvider) ID() string                         { return p.legacy.ID() }
func (p protocolProvider) Label() string                      { return p.legacy.Label() }
func (p protocolProvider) Capability() Capability             { return p.legacy.Capability() }
func (p protocolProvider) Normalize(a string) (string, error) { return p.legacy.Normalize(a) }
func (p protocolProvider) CanHandle(a string) bool            { return p.legacy.CanHandle(a) }
func (p protocolProvider) Fetch(ctx context.Context, a string) (string, error) {
	return p.legacy.Fetch(ctx, a)
}

// Group preserves the optional provider grouping capability across the
// protocol adapter. Returning an empty group is the established fallback.
func (p protocolProvider) Group(address string) string {
	if g, ok := p.legacy.(interface{ Group(string) string }); ok {
		return g.Group(address)
	}
	return ""
}

func (p protocolProvider) Descriptor() PluginDescriptor {
	cap := p.Capability()
	return PluginDescriptor{Version: pluginProtocolVersion, Kind: PluginKindProvider, ID: p.ID(), Label: p.Label(), Description: cap.Description}
}

func (p protocolProvider) Handle(ctx context.Context, req PluginRequest) (PluginResponse, error) {
	switch req.Action {
	case "describe":
		return PluginResponse{Descriptor: p.Descriptor()}, nil
	case "capability":
		return PluginResponse{Capability: p.Capability()}, nil
	case "can_handle":
		result := p.CanHandle(req.Address)
		return PluginResponse{Result: &result}, nil
	case "normalize":
		address, err := p.Normalize(req.Address)
		return PluginResponse{Address: address}, err
	case "fetch":
		path, err := p.Fetch(ctx, req.Address)
		return PluginResponse{Path: path}, err
	default:
		return PluginResponse{}, unsupportedPluginAction(p.Descriptor(), req.Action)
	}
}
