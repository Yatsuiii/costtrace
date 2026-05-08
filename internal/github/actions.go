package github

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/Yatsuiii/costtrace/internal/storage"
	gh "github.com/google/go-github/v72/github"
)

type Client struct {
	gh      *gh.Client
	owner   string
	repo    string
	pattern *regexp.Regexp
}

func NewClient(token, repoSlug, workflowPattern string) (*Client, error) {
	parts := strings.SplitN(repoSlug, "/", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("repo must be owner/name, got %q", repoSlug)
	}
	re, err := regexp.Compile(workflowPattern)
	if err != nil {
		return nil, fmt.Errorf("workflow pattern: %w", err)
	}
	var c *gh.Client
	if token != "" {
		c = gh.NewClient(nil).WithAuthToken(token)
	} else {
		c = gh.NewClient(nil)
	}
	return &Client{gh: c, owner: parts[0], repo: parts[1], pattern: re}, nil
}

func (c *Client) FetchRuns(ctx context.Context, since time.Time) ([]storage.DeployRow, error) {
	opts := &gh.ListWorkflowRunsOptions{
		Status:      "completed",
		Created:     ">=" + since.Format("2006-01-02"),
		ListOptions: gh.ListOptions{PerPage: 100},
	}

	var rows []storage.DeployRow
	for {
		runs, resp, err := c.gh.Actions.ListRepositoryWorkflowRuns(ctx, c.owner, c.repo, opts)
		if err != nil {
			return nil, fmt.Errorf("list workflow runs: %w", err)
		}
		for _, r := range runs.WorkflowRuns {
			if !c.pattern.MatchString(r.GetName()) {
				continue
			}
			var prNum *int
			if len(r.PullRequests) > 0 {
				n := r.PullRequests[0].GetNumber()
				prNum = &n
			}
			rows = append(rows, storage.DeployRow{
				ID:          fmt.Sprintf("gha-%d", r.GetID()),
				Repo:        c.owner + "/" + c.repo,
				Branch:      r.GetHeadBranch(),
				CommitSHA:   r.GetHeadSHA(),
				PRNumber:    prNum,
				Title:       r.GetName(),
				StartedAt:   r.GetRunStartedAt().Time,
				CompletedAt: r.GetUpdatedAt().Time,
				Status:      r.GetConclusion(),
			})
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return rows, nil
}
