package wtc

// Approval is bound to the definition as well as its path. A different provider,
// command, access policy or destination needs a fresh explicit choice.
type runtimeTunnelIntent struct {
	On         bool   `json:"on"`
	Definition string `json:"definition"`
}

type runtimeTunnelChoice struct{ runtimeTunnelIntent }

func runtimeTunnelLifecycle(items []RuntimeItem) {
	for i := range items {
		item := &items[i]
		if item.Kind != "endpoint" {
			continue
		}
		if item.TunnelIntent == "stale" {
			item.Lifecycle = "tunnel declaration changed; choose the tunnel again"
		} else if item.TunnelIntent == "on" {
			switch item.State {
			case "ready", "running", "starting", "checking":
			default:
				item.Lifecycle = "tunnel intended on but not running"
			}
		}
	}
}
