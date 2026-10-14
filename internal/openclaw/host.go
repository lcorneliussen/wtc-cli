package openclaw

// HostKind is what wtc is running under.
type HostKind string

const (
	HostOpenClaw HostKind = "openclaw"
	HostHerdr    HostKind = "herdr"
	HostTerminal HostKind = "terminal"
)

// DetectHost reads the host from the environment. OpenClaw sets
// OPENCLAW_SHELL=exec for every command an agent runs, and it wins: a gateway
// started from a herdr pane still runs its agents' commands under OpenClaw.
func DetectHost(getenv func(string) string) HostKind {
	if getenv("OPENCLAW_SHELL") == "exec" {
		return HostOpenClaw
	}
	for _, name := range []string{"HERDR_SESSION", "HERDR_WORKSPACE_ID", "HARNESS_HERDR_SESSION"} {
		if getenv(name) != "" {
			return HostHerdr
		}
	}
	return HostTerminal
}

// Plan is what a wtc command prints under OpenClaw instead of acting: the
// intents for the agent to carry out, in order, and what it should know
// before it does.
type Plan struct {
	Host    string   `json:"host"`
	Version int      `json:"version"`
	Intents []Intent `json:"intents"`
	// Pending are the uncommitted paths a publication will commit.
	Pending  []string `json:"pending,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

// NewPlan wraps intents for printing.
func NewPlan(intents ...Intent) Plan {
	return Plan{Host: Host, Version: ContractVersion, Intents: intents}
}
