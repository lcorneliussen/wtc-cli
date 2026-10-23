package openclaw

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"unicode/utf16"
)

// Proposal is the title and description the agent proposes for the pull
// request of a checkout's branch. The person's push from the collection card
// opens the pull request with them; the gateway takes them only when it
// opens one, not on later pushes.
type Proposal struct {
	Branch string `json:"branch"`
	Title  string `json:"title,omitempty"`
	Body   string `json:"body,omitempty"`
}

// The gateway's limits for sessions.github.publish, in UTF-16 code units as
// JavaScript counts them.
const (
	MaxProposalTitle = 256
	MaxProposalBody  = 8192
)

// proposalFile is kept in the worktree's own git directory, so it is never
// committed and goes with the worktree.
const proposalFile = "wtc-pull-request.json"

func proposalPath(dir string) (string, error) {
	return git(dir, "rev-parse", "--path-format=absolute", "--git-path", proposalFile)
}

// CheckProposal refuses a title or description the gateway would refuse.
func CheckProposal(title, body string) error {
	if strings.ContainsAny(title, "\r\n") {
		return errors.New("the title is one line")
	}
	if n := len(utf16.Encode([]rune(title))); n > MaxProposalTitle {
		return fmt.Errorf("the title has %d characters; the gateway takes at most %d", n, MaxProposalTitle)
	}
	if n := len(utf16.Encode([]rune(body))); n > MaxProposalBody {
		return fmt.Errorf("the description has %d characters; the gateway takes at most %d", n, MaxProposalBody)
	}
	return nil
}

// ReadProposal is what was proposed for the branch the checkout at dir is
// on, or nil.
func ReadProposal(dir, branch string) *Proposal {
	p, err := proposalPath(dir)
	if err != nil || branch == "" {
		return nil
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	var proposal Proposal
	if json.Unmarshal(data, &proposal) != nil || proposal.Branch != branch || proposal.Title == "" && proposal.Body == "" {
		return nil
	}
	return &proposal
}

// Propose records the title and description for the checkout's branch. An
// empty one keeps what was proposed before.
func (c *Checkout) Propose(title, body string) (*Proposal, error) {
	title, body = strings.TrimSpace(title), strings.TrimSpace(body)
	if err := CheckProposal(title, body); err != nil {
		return nil, err
	}
	proposal := Proposal{Branch: c.Branch()}
	if old := ReadProposal(c.Dir, proposal.Branch); old != nil {
		proposal = *old
	}
	if title != "" {
		proposal.Title = title
	}
	if body != "" {
		proposal.Body = body
	}
	if proposal.Title == "" && proposal.Body == "" {
		return nil, nil
	}
	p, err := proposalPath(c.Dir)
	if err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(proposal, "", "  ")
	if err != nil {
		return nil, err
	}
	return &proposal, os.WriteFile(p, append(data, '\n'), 0o644)
}
