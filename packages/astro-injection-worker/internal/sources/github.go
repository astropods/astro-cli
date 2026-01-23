package sources

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/google/go-github/v57/github"
	"golang.org/x/oauth2"
)

// GitHubSource fetches data from GitHub
type GitHubSource struct {
	client     *github.Client
	httpClient *http.Client
	token      string
	repo       string
	owner      string
}

// Document represents a fetched document
type Document struct {
	ID      string
	Content string
	Metadata map[string]string
}

// NewGitHubSource creates a new GitHub source
func NewGitHubSource(token string, repo string) (*GitHubSource, error) {
	// Parse owner/repo
	parts := strings.Split(repo, "/")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid repo format, expected owner/repo")
	}

	// Create client (authenticated if token provided, otherwise unauthenticated)
	ctx := context.Background()
	var client *github.Client
	var httpClient *http.Client
	if token != "" {
		ts := oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: token},
		)
		tc := oauth2.NewClient(ctx, ts)
		client = github.NewClient(tc)
		httpClient = tc
	} else {
		// Use unauthenticated client for public repos
		client = github.NewClient(nil)
		httpClient = http.DefaultClient
	}

	return &GitHubSource{
		client:     client,
		httpClient: httpClient,
		token:      token,
		owner:      parts[0],
		repo:       parts[1],
	}, nil
}

// FetchIssues fetches all issues from the repository
func (s *GitHubSource) FetchIssues(ctx context.Context) ([]Document, error) {
	var allDocs []Document
	pageCount := 0
	issueCount := 0

	// Use smaller page size to avoid hitting pagination limits
	opts := &github.IssueListByRepoOptions{
		State: "all",
		ListOptions: github.ListOptions{
			PerPage: 100,
		},
	}

	fmt.Printf("Fetching issues from %s/%s...\n", s.owner, s.repo)

	for {
		issues, resp, err := s.client.Issues.ListByRepo(ctx, s.owner, s.repo, opts)
		if err != nil {
			// Check if this is a pagination error
			if strings.Contains(err.Error(), "422") && strings.Contains(err.Error(), "cursor based pagination") {
				fmt.Printf("Note: Repository has too many issues for page-based pagination\n")
				fmt.Printf("Fetched %d issues before pagination limit\n", issueCount)
				// Return what we have so far
				return allDocs, nil
			}
			return nil, fmt.Errorf("failed to fetch issues: %w", err)
		}

		pageCount++
		pageIssues := 0

		for _, issue := range issues {
			// Skip pull requests (GitHub API returns them as issues)
			if issue.PullRequestLinks != nil {
				continue
			}

			doc := Document{
				ID:      fmt.Sprintf("issue-%d", *issue.Number),
				Content: fmt.Sprintf("# %s\n\n%s", *issue.Title, issue.GetBody()),
				Metadata: map[string]string{
					"number":     fmt.Sprintf("%d", *issue.Number),
					"title":      *issue.Title,
					"state":      *issue.State,
					"created_at": issue.GetCreatedAt().Format("2006-01-02"),
					"url":        *issue.HTMLURL,
				},
			}

			if issue.User != nil {
				doc.Metadata["author"] = issue.GetUser().GetLogin()
			}

			allDocs = append(allDocs, doc)
			pageIssues++
			issueCount++
		}

		// Log progress after each page
		if resp.NextPage == 0 {
			fmt.Printf("Fetched page %d: %d issues (total: %d issues, complete)\n",
				pageCount, pageIssues, issueCount)
			break
		} else {
			fmt.Printf("Fetched page %d: %d issues (total: %d issues so far, more pages...)\n",
				pageCount, pageIssues, issueCount)
		}

		// Check rate limit
		if resp.Rate.Remaining < 10 {
			fmt.Printf("Warning: GitHub API rate limit low (remaining: %d, resets at: %v)\n",
				resp.Rate.Remaining, resp.Rate.Reset.Time)
		}

		// Check if we've reached a reasonable limit to avoid pagination issues
		if pageCount >= 100 {
			fmt.Printf("Note: Reached page limit (%d pages, %d issues fetched)\n", pageCount, issueCount)
			fmt.Printf("Repository may have more issues, but stopping to avoid pagination errors\n")
			break
		}

		opts.Page = resp.NextPage
	}

	return allDocs, nil
}

// FetchPageWithCursor fetches a single page of issues using cursor-based pagination (for backfill)
func (s *GitHubSource) FetchPageWithCursor(ctx context.Context, cursor string) ([]Document, string, bool, error) {
	fmt.Printf("Fetching issues from %s/%s (backfill mode, cursor: %s)...\n", s.owner, s.repo, cursor)

	// Prepare GraphQL query for cursor-based pagination (ordered by CREATED_AT for backfill)
	query := `
		query($owner: String!, $name: String!, $cursor: String) {
			repository(owner: $owner, name: $name) {
				issues(first: 100, after: $cursor, orderBy: {field: CREATED_AT, direction: ASC}) {
					nodes {
						number
						title
						body
						state
						createdAt
						url
						author {
							login
						}
					}
					pageInfo {
						hasNextPage
						endCursor
					}
				}
			}
		}
	`

	variables := map[string]interface{}{
		"owner": s.owner,
		"name":  s.repo,
	}
	if cursor != "" {
		variables["cursor"] = cursor
	}

	payload := map[string]interface{}{
		"query":     query,
		"variables": variables,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, "", false, fmt.Errorf("failed to marshal GraphQL query: %w", err)
	}

	// Make GraphQL API request
	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.github.com/graphql", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, "", false, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if s.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, "", false, fmt.Errorf("failed to execute GraphQL request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, "", false, fmt.Errorf("GraphQL request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var result struct {
		Data struct {
			Repository struct {
				Issues struct {
					Nodes []struct {
						Number    int    `json:"number"`
						Title     string `json:"title"`
						Body      string `json:"body"`
						State     string `json:"state"`
						CreatedAt string `json:"createdAt"`
						URL       string `json:"url"`
						Author    struct {
							Login string `json:"login"`
						} `json:"author"`
					} `json:"nodes"`
					PageInfo struct {
						HasNextPage bool   `json:"hasNextPage"`
						EndCursor   string `json:"endCursor"`
					} `json:"pageInfo"`
				} `json:"issues"`
			} `json:"repository"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, "", false, fmt.Errorf("failed to decode GraphQL response: %w", err)
	}

	if len(result.Errors) > 0 {
		return nil, "", false, fmt.Errorf("GraphQL errors: %v", result.Errors)
	}

	// Convert to documents
	var docs []Document
	for _, issue := range result.Data.Repository.Issues.Nodes {
		doc := Document{
			ID:      fmt.Sprintf("issue-%d", issue.Number),
			Content: fmt.Sprintf("# %s\n\n%s", issue.Title, issue.Body),
			Metadata: map[string]string{
				"number":     fmt.Sprintf("%d", issue.Number),
				"title":      issue.Title,
				"state":      issue.State,
				"created_at": issue.CreatedAt,
				"url":        issue.URL,
			},
		}

		if issue.Author.Login != "" {
			doc.Metadata["author"] = issue.Author.Login
		}

		docs = append(docs, doc)
	}

	nextCursor := result.Data.Repository.Issues.PageInfo.EndCursor
	hasMore := result.Data.Repository.Issues.PageInfo.HasNextPage

	fmt.Printf("Fetched %d issues, hasMore=%v, nextCursor=%s\n", len(docs), hasMore, nextCursor)
	return docs, nextCursor, hasMore, nil
}

// FetchIncrementalWithCursor fetches issues updated since a timestamp (for incremental syncs)
func (s *GitHubSource) FetchIncrementalWithCursor(ctx context.Context, since string, cursor string) ([]Document, string, bool, error) {
	fmt.Printf("Fetching issues from %s/%s (incremental mode, since: %s, cursor: %s)...\n", s.owner, s.repo, since, cursor)

	// Prepare GraphQL query for incremental updates (ordered by UPDATED_AT descending to get newest first)
	query := `
		query($owner: String!, $name: String!, $cursor: String) {
			repository(owner: $owner, name: $name) {
				issues(first: 100, after: $cursor, orderBy: {field: UPDATED_AT, direction: DESC}) {
					nodes {
						number
						title
						body
						state
						createdAt
						updatedAt
						url
						author {
							login
						}
					}
					pageInfo {
						hasNextPage
						endCursor
					}
				}
			}
		}
	`

	variables := map[string]interface{}{
		"owner": s.owner,
		"name":  s.repo,
	}
	if cursor != "" {
		variables["cursor"] = cursor
	}

	payload := map[string]interface{}{
		"query":     query,
		"variables": variables,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, "", false, fmt.Errorf("failed to marshal GraphQL query: %w", err)
	}

	// Make GraphQL API request
	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.github.com/graphql", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, "", false, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if s.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, "", false, fmt.Errorf("failed to execute GraphQL request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, "", false, fmt.Errorf("GraphQL request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var result struct {
		Data struct {
			Repository struct {
				Issues struct {
					Nodes []struct {
						Number    int    `json:"number"`
						Title     string `json:"title"`
						Body      string `json:"body"`
						State     string `json:"state"`
						CreatedAt string `json:"createdAt"`
						UpdatedAt string `json:"updatedAt"`
						URL       string `json:"url"`
						Author    struct {
							Login string `json:"login"`
						} `json:"author"`
					} `json:"nodes"`
					PageInfo struct {
						HasNextPage bool   `json:"hasNextPage"`
						EndCursor   string `json:"endCursor"`
					} `json:"pageInfo"`
				} `json:"issues"`
			} `json:"repository"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, "", false, fmt.Errorf("failed to decode GraphQL response: %w", err)
	}

	if len(result.Errors) > 0 {
		return nil, "", false, fmt.Errorf("GraphQL errors: %v", result.Errors)
	}

	// Convert to documents and filter by timestamp
	var docs []Document
	for _, issue := range result.Data.Repository.Issues.Nodes {
		// Stop if we've reached issues older than our last sync
		if issue.UpdatedAt <= since {
			fmt.Printf("Reached issues older than last sync (%s), stopping incremental fetch\n", since)
			return docs, "", false, nil
		}

		doc := Document{
			ID:      fmt.Sprintf("issue-%d", issue.Number),
			Content: fmt.Sprintf("# %s\n\n%s", issue.Title, issue.Body),
			Metadata: map[string]string{
				"number":     fmt.Sprintf("%d", issue.Number),
				"title":      issue.Title,
				"state":      issue.State,
				"created_at": issue.CreatedAt,
				"updated_at": issue.UpdatedAt,
				"url":        issue.URL,
			},
		}

		if issue.Author.Login != "" {
			doc.Metadata["author"] = issue.Author.Login
		}

		docs = append(docs, doc)
	}

	nextCursor := result.Data.Repository.Issues.PageInfo.EndCursor
	hasMore := result.Data.Repository.Issues.PageInfo.HasNextPage

	fmt.Printf("Fetched %d new/updated issues, hasMore=%v, nextCursor=%s\n", len(docs), hasMore, nextCursor)
	return docs, nextCursor, hasMore, nil
}

// FetchPage fetches a single page of issues for incremental processing
func (s *GitHubSource) FetchPage(ctx context.Context, page int) ([]Document, bool, error) {
	fmt.Printf("Fetching issues page %d from %s/%s...\n", page, s.owner, s.repo)

	opts := &github.IssueListByRepoOptions{
		State: "all",
		ListOptions: github.ListOptions{
			PerPage: 100,
			Page:    page,
		},
	}

	issues, resp, err := s.client.Issues.ListByRepo(ctx, s.owner, s.repo, opts)
	if err != nil {
		// Check if this is a pagination error
		if strings.Contains(err.Error(), "422") && strings.Contains(err.Error(), "cursor based pagination") {
			fmt.Printf("Note: Repository has too many issues for page-based pagination at page %d\n", page)
			fmt.Printf("Returning no more results\n")
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("failed to fetch issues: %w", err)
	}

	var docs []Document
	for _, issue := range issues {
		// Skip pull requests (GitHub API returns them as issues)
		if issue.PullRequestLinks != nil {
			continue
		}

		doc := Document{
			ID:      fmt.Sprintf("issue-%d", *issue.Number),
			Content: fmt.Sprintf("# %s\n\n%s", *issue.Title, issue.GetBody()),
			Metadata: map[string]string{
				"number":     fmt.Sprintf("%d", *issue.Number),
				"title":      *issue.Title,
				"state":      *issue.State,
				"created_at": issue.GetCreatedAt().Format("2006-01-02"),
				"url":        *issue.HTMLURL,
			},
		}

		if issue.User != nil {
			doc.Metadata["author"] = issue.GetUser().GetLogin()
		}

		docs = append(docs, doc)
	}

	// Check if there are more pages
	hasMore := resp.NextPage != 0 && len(issues) == opts.PerPage

	// Log rate limit
	if resp.Rate.Remaining < 10 {
		fmt.Printf("Warning: GitHub API rate limit low (remaining: %d, resets at: %v)\n",
			resp.Rate.Remaining, resp.Rate.Reset.Time)
	}

	// Check pagination limit
	if page >= 100 {
		fmt.Printf("Note: Reached page limit (%d pages)\n", page)
		hasMore = false
	}

	fmt.Printf("Fetched page %d: %d issues, hasMore=%v\n", page, len(docs), hasMore)
	return docs, hasMore, nil
}
