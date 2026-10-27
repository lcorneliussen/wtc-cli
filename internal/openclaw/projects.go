package openclaw

import (
	"errors"
	"net/http"
)

// DiscoveryProject is the read-only project metadata available to a session.
type DiscoveryProject struct {
	Name        string              `json:"name"`
	Harness     string              `json:"harness"`
	Ref         string              `json:"ref"`
	Default     bool                `json:"default,omitempty"`
	Repos       []ProjectRepository `json:"repos"`
	Error       string              `json:"error,omitempty"`
	ProjectID   string              `json:"project_id,omitempty"`
	HasWorktree bool                `json:"has_worktree,omitempty"`
}

type ProjectRepository struct {
	Name       string `json:"name"`
	Remote     string `json:"remote"`
	Role       string `json:"role,omitempty"`
	DefaultRef string `json:"default_ref,omitempty"`
	ProjectID  string `json:"project_id,omitempty"`
}

// SandboxProjects contains only operator-allowed metadata. It grants no Git
// access or permission to create worktrees; the gateway still authorizes those.
type SandboxProjects struct {
	Projects          []DiscoveryProject `json:"projects"`
	Collection        string             `json:"collection,omitempty"`
	CollectionProject string             `json:"collection_project,omitempty"`
}

func (g *HTTPGateway) Projects() (SandboxProjects, error) {
	var p SandboxProjects
	err := g.do(http.MethodGet, "/wtc/projects", nil, &p)
	if err == nil && p.Projects == nil {
		err = errors.New("gateway project metadata unavailable: missing projects list")
	}
	if err == nil {
		for _, project := range p.Projects {
			if project.Name == "" || project.Harness == "" || project.Ref == "" || project.Repos == nil {
				err = errors.New("gateway project metadata unavailable: incomplete project")
				break
			}
		}
	}
	return p, err
}
