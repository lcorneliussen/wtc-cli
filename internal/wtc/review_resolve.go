package wtc

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
)

type ReviewResolveOptions struct {
	Reply   string
	File    string
	Line    int
	Concern string
	Key     string
}

type ReviewResolveResult struct {
	Resolved int `json:"resolved"`
}

func (c *Context) ResolveReviewBundle(bundle string, opt ReviewResolveOptions) (ReviewResolveResult, error) {
	bundle, err := filepath.Abs(bundle)
	if err != nil {
		return ReviewResolveResult{}, err
	}
	manifest, err := readReviewManifest(bundle)
	if err != nil {
		return ReviewResolveResult{}, err
	}
	worktree, slug, forge, err := c.ReviewRepo(manifest.Repo)
	if err != nil {
		return ReviewResolveResult{}, err
	}
	if worktree != manifest.RepoDir || slug != manifest.Slug || forge != manifest.Forge {
		return ReviewResolveResult{}, fmt.Errorf("bundle repository does not match this collection")
	}
	rows := readInlineRecords(bundle)
	result := ReviewResolveResult{}
	for i := range rows {
		row := &rows[i]
		if row.ID == "" || row.Resolved || opt.Key != "" && row.Key != opt.Key || opt.Concern != "" && row.Concern != opt.Concern || opt.File != "" && row.File != opt.File || opt.Line != 0 && row.Line != opt.Line {
			continue
		}
		if !prNumber.MatchString(row.ID) {
			return result, fmt.Errorf("invalid inline comment id for %s", row.Key)
		}
		if opt.Reply != "" {
			if err := replyReviewInline(manifest, row.ID, opt.Reply); err != nil {
				return result, err
			}
		}
		if err := resolveReviewInline(manifest, row.ID); err != nil {
			return result, err
		}
		row.Resolved = true
		row.Error = ""
		if err := writeInlineRecords(bundle, rows); err != nil {
			return result, err
		}
		result.Resolved++
	}
	return result, nil
}

func replyReviewInline(m ReviewManifest, id, body string) error {
	if m.Forge == "github" {
		num, _ := strconv.Atoi(id)
		payload, _ := json.Marshal(map[string]any{"body": body, "in_reply_to": num})
		_, err := reviewForgeCommand(m.RepoDir, "gh", []string{"api", "-X", "POST", fmt.Sprintf("repos/%s/pulls/%s/comments", m.Slug, m.PR), "--input", "-"}, payload)
		return err
	}
	_, err := reviewForgeCommand(m.RepoDir, "bb", []string{"pr", "comments", "reply", m.PR, id, body}, nil)
	return err
}

func resolveReviewInline(m ReviewManifest, id string) error {
	if m.Forge == "bitbucket" {
		_, err := reviewForgeCommand(m.RepoDir, "bb", []string{"pr", "comments", "resolve", m.PR, id}, nil)
		return err
	}
	parts := splitReviewSlug(m.Slug)
	if len(parts) != 2 {
		return fmt.Errorf("invalid GitHub repository slug")
	}
	query := `query($o:String!,$n:String!,$num:Int!){repository(owner:$o,name:$n){pullRequest(number:$num){reviewThreads(first:100){nodes{id isResolved comments(first:50){nodes{databaseId}}}}}}}`
	out, err := reviewForgeCommand(m.RepoDir, "gh", []string{"api", "graphql", "-f", "query=" + query, "-f", "o=" + parts[0], "-f", "n=" + parts[1], "-F", "num=" + m.PR}, nil)
	if err != nil {
		return err
	}
	var graph struct {
		Data struct {
			Repository struct {
				PullRequest struct {
					ReviewThreads struct {
						Nodes []struct {
							ID         string `json:"id"`
							IsResolved bool   `json:"isResolved"`
							Comments   struct {
								Nodes []struct {
									DatabaseID int64 `json:"databaseId"`
								} `json:"nodes"`
							} `json:"comments"`
						} `json:"nodes"`
					} `json:"reviewThreads"`
				} `json:"pullRequest"`
			} `json:"repository"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &graph); err != nil {
		return err
	}
	target, _ := strconv.ParseInt(id, 10, 64)
	for _, thread := range graph.Data.Repository.PullRequest.ReviewThreads.Nodes {
		for _, comment := range thread.Comments.Nodes {
			if comment.DatabaseID != target {
				continue
			}
			if thread.IsResolved {
				return nil
			}
			mutation := `mutation($id:ID!){resolveReviewThread(input:{threadId:$id}){thread{isResolved}}}`
			_, err := reviewForgeCommand(m.RepoDir, "gh", []string{"api", "graphql", "-f", "query=" + mutation, "-f", "id=" + thread.ID}, nil)
			return err
		}
	}
	return fmt.Errorf("GitHub review thread for comment %s not found", id)
}

func splitReviewSlug(slug string) []string {
	for i := 0; i < len(slug); i++ {
		if slug[i] == '/' {
			return []string{slug[:i], slug[i+1:]}
		}
	}
	return nil
}
