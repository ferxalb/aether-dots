package github

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"unicode"
)

const (
	maxSearchTerms     = 8
	maxSearchTermBytes = 40
	maxSearchLimit     = 5
)

// SearchTerms reduces model-supplied text to plain search words. Every
// qualifier (anything with ':', such as repo: or org:), operator, quote and
// symbol is dropped, so a query can never widen the search beyond the one
// repository SearchIssues pins.
func SearchTerms(query string) []string {
	var terms []string
	for _, field := range strings.Fields(query) {
		if strings.Contains(field, ":") {
			continue
		}
		word := strings.Map(func(r rune) rune {
			if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' || r == '.' {
				return r
			}
			return -1
		}, field)
		word = strings.Trim(word, "-.")
		switch strings.ToUpper(word) {
		case "", "AND", "OR", "NOT":
			continue
		}
		if len(word) > maxSearchTermBytes {
			word = word[:maxSearchTermBytes]
		}
		terms = append(terms, word)
		if len(terms) == maxSearchTerms {
			break
		}
	}
	return terms
}

// SearchIssues finds open and closed issues (not pull requests) in exactly one
// repository whose text matches the plain words in query.
func (c *APIClient) SearchIssues(ctx context.Context, owner, repo, query string, limit int) ([]Issue, error) {
	if limit < 1 || limit > maxSearchLimit {
		return nil, errors.New("issue search limit must be from 1 to 5")
	}
	if !validName(owner) || !validName(repo) {
		return nil, errors.New("repository name is invalid")
	}
	terms := SearchTerms(query)
	if len(terms) == 0 {
		return nil, errors.New("issue search needs at least one plain word")
	}
	q := "repo:" + owner + "/" + repo + " is:issue " + strings.Join(terms, " ")
	values := url.Values{"q": {q}, "per_page": {strconv.Itoa(limit)}, "page": {"1"}}
	var result struct {
		Items []Issue `json:"items"`
	}
	if err := c.getJSON(ctx, "/search/issues", values, &result); err != nil {
		return nil, err
	}
	issues := make([]Issue, 0, len(result.Items))
	for _, issue := range result.Items {
		if issue.PullRequest != nil {
			continue
		}
		item := cleanIssue(issue)
		item.Body = safeExternalText(item.Body, 300)
		issues = append(issues, item)
		if len(issues) == limit {
			break
		}
	}
	return issues, nil
}

func (s *Service) SearchIssues(ctx context.Context, owner, repo, query string, limit int) ([]Issue, error) {
	var result []Issue
	err := s.withClient(ctx, func(ctx context.Context, client *APIClient) error {
		var err error
		result, err = client.SearchIssues(ctx, owner, repo, query, limit)
		return err
	})
	return result, err
}
