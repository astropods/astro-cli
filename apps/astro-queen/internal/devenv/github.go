package devenv

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/google/go-github/v69/github"
	"golang.org/x/oauth2"
)

type Client struct {
	gh    *github.Client
	owner string
	repo  string
}

func NewClient(owner, repo string) (*Client, error) {
	token, err := getToken()
	if err != nil {
		return nil, err
	}
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	tc := oauth2.NewClient(context.Background(), ts)
	return &Client{
		gh:    github.NewClient(tc),
		owner: owner,
		repo:  repo,
	}, nil
}

func getToken() (string, error) {
	out, err := exec.Command("gh", "auth", "token").Output()
	if err == nil {
		t := strings.TrimSpace(string(out))
		if t != "" {
			return t, nil
		}
	}
	return "", fmt.Errorf("could not get GitHub token: install gh CLI and run 'gh auth login', or set GITHUB_TOKEN")
}

func (c *Client) CurrentUser(ctx context.Context) (string, error) {
	user, _, err := c.gh.Users.Get(ctx, "")
	if err != nil {
		return "", fmt.Errorf("getting current user: %w", err)
	}
	return user.GetLogin(), nil
}

const (
	workflowFile = "dev-env-provision.yml"
)

func (c *Client) DispatchWorkflow(ctx context.Context, name, action string) error {
	_, err := c.gh.Actions.CreateWorkflowDispatchEventByFileName(ctx, c.owner, c.repo, workflowFile,
		github.CreateWorkflowDispatchEventRequest{
			Ref: "main",
			Inputs: map[string]interface{}{
				"developer_name": name,
				"action":         action,
			},
		})
	return err
}

type FoundRun struct {
	ID      int64
	HTMLURL string
}

func (c *Client) FindWorkflowRun(ctx context.Context, name, action string) (*FoundRun, error) {
	cutoff := time.Now().Add(-60 * time.Second)

	for attempt := range 10 {
		if attempt > 0 {
			time.Sleep(5 * time.Second)
		}

		runs, _, err := c.gh.Actions.ListWorkflowRunsByFileName(ctx, c.owner, c.repo, workflowFile,
			&github.ListWorkflowRunsOptions{
				Branch: "main",
				Event:  "workflow_dispatch",
				ListOptions: github.ListOptions{
					PerPage: 5,
				},
			})
		if err != nil {
			return nil, fmt.Errorf("listing workflow runs: %w", err)
		}

		for _, run := range runs.WorkflowRuns {
			if run.CreatedAt.Before(cutoff) {
				continue
			}
			return &FoundRun{ID: run.GetID(), HTMLURL: run.GetHTMLURL()}, nil
		}
	}
	return nil, fmt.Errorf("could not find workflow run after 10 attempts (50s)")
}

type JobStatus struct {
	Name       string
	Status     string
	Conclusion string
}

type RunStatus struct {
	Status     string
	Conclusion string
	HTMLURL    string
	Jobs       []JobStatus
}

func (c *Client) PollWorkflowRun(ctx context.Context, runID int64, callback func(RunStatus)) error {
	for {
		run, _, err := c.gh.Actions.GetWorkflowRunByID(ctx, c.owner, c.repo, runID)
		if err != nil {
			return fmt.Errorf("getting workflow run: %w", err)
		}

		jobsResp, _, err := c.gh.Actions.ListWorkflowJobs(ctx, c.owner, c.repo, runID, &github.ListWorkflowJobsOptions{})
		if err != nil {
			return fmt.Errorf("listing workflow jobs: %w", err)
		}

		rs := RunStatus{
			Status:     run.GetStatus(),
			Conclusion: run.GetConclusion(),
			HTMLURL:    run.GetHTMLURL(),
		}
		for _, j := range jobsResp.Jobs {
			rs.Jobs = append(rs.Jobs, JobStatus{
				Name:       j.GetName(),
				Status:     j.GetStatus(),
				Conclusion: j.GetConclusion(),
			})
		}

		callback(rs)

		if rs.Status == "completed" {
			if rs.Conclusion == "success" {
				return nil
			}
			return fmt.Errorf("workflow run failed: %s — %s", rs.Conclusion, rs.HTMLURL)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(15 * time.Second):
		}
	}
}
