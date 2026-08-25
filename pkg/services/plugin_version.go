package services

// pluginProtocolVersion is the Target/Provider plugin protocol version skm
// itself implements. Plugins declare the version in their `id` response
// ("protocol_version"), defaulting to v1 when absent so older plugins keep
// loading; a trailing version gets a startup warning and v2-only actions
// (remove_foreign) a clear error.
const pluginProtocolVersion = 2

// removeForeignProtocolVersion is the minimum Target plugin protocol version
// that implements the remove_foreign action (conflict cleanup).
const removeForeignProtocolVersion = 2
