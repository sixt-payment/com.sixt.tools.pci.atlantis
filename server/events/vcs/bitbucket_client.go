package vcs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"

	"github.com/hootsuite/atlantis/server/events/models"
	bitbucket "github.com/ktrysmt/go-bitbucket"
)

type bitbucketCommitStatus struct {
	State       string
	Key         string
	Name        string
	URL         string
	Description string
}

type bitbucketPullReviewers struct {
	Username string
}

type bitbucketPullApprovers struct {
	Values []bitbucketPullReviewers
}

type bitbucketAPIError struct {
	Message    string `json:"error,omitempty"`
	Type       string `json:"type,omitempty"`
	StatusCode int
	Endpoint   string
}

func (e bitbucketAPIError) Error() string {
	return fmt.Sprintf("Error (%d) on %s: %s", e.StatusCode, e.Endpoint, e.Message)
}

// BitbucketClient is a client for the bitbucket.org API
type BitbucketClient struct {
	username string
	password string
	client   *http.Client
	ctx      context.Context
}

func (b *BitbucketClient) do(method, endpoint string, payload *bytes.Buffer) (*http.Response, error) {
	baseURL := "https://api.bitbucket.org/2.0/"
	requestURL := fmt.Sprintf("%s/%s", baseURL, endpoint)

	var bodyreader io.Reader

	req, err := http.NewRequest(method, requestURL, bodyreader)
	if err != nil {
		return nil, err
	}

	req.SetBasicAuth(b.username, b.password)
	if payload != nil {
		req.Header.Add("Content-Type", "application/json")
	}
	req.Close = true

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 || resp.StatusCode < 200 {
		apiError := bitbucketAPIError{
			StatusCode: resp.StatusCode,
			Endpoint:   endpoint,
		}

		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}

		if json.Unmarshal(body, &apiError) != nil {
			apiError.Message = string(body)
		}

		return resp, error(apiError)
	}

	return resp, err
}

// NewBitbucketClient returns a valid GitHub client.
func NewBitbucketClient(user string, pass string) (*BitbucketClient, error) {
	return &BitbucketClient{
		username: user,
		password: pass,
		client:   &http.Client{},
		ctx:      context.Background(),
	}, nil
}

// GetModifiedFiles returns the names of files that were modified in the pull request.
// The names include the path to the file from the repo root, ex. parent/child/file.txt.
func (b *BitbucketClient) GetModifiedFiles(repo models.Repo, pull models.PullRequest) ([]string, error) {
	return nil, nil
}

// CreateComment creates a comment on the pull request.
func (b *BitbucketClient) CreateComment(repo models.Repo, pull models.PullRequest, comment string) error {
	return nil
}

// PullIsApproved returns true if the pull request was approved.
func (b *BitbucketClient) PullIsApproved(repo models.Repo, pull models.PullRequest) (bool, error) {
	// /2.0/repositories/bitbucket/bitbucket/pullrequests?fields=values.id,values.reviewers.username,values.state&q=id=
	pullRequestURL := fmt.Sprintf("repositories/%s/%s/pullrequests?fields=values.id,values.reviewers.approved&q=id=%d", repo.Owner, repo.Name, pull.Num)
	resp, err := b.do("GET", pullRequestURL, nil)
	if err != nil {
		return false, err
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return false, err
	}

	pullRequest := map[string]interface{}{}

	if err := json.Unmarshal([]byte(body), &pullRequest); err != nil {
		return false, err
	}

	return false, nil
}

// UpdateStatus updates the build status of a commit.
func (b *BitbucketClient) UpdateStatus(repo models.Repo, pull models.PullRequest, state CommitStatus,
	description string) error {

	const statusContext = "Atlantis"
	bbState := "FAILED"

	switch state {
	case Pending:
		bbState = "INPROGRESS"
	case Success:
		bbState = "SUCCESSFUL"
	case Failed:
		bbState = "FAILED"
	}

	status := bitbucketCommitStatus{
		Name:        "Atlantis",
		State:       bbState,
		Key:         "FIXME",
		URL:         fmt.Sprintf("localhost:4141/bla"),
		Description: description,
	}

	payload := new(bytes.Buffer)
	err := json.NewEncoder(payload).Encode(status)
	if err != nil {
		return err
	}

	commitStatusURL := fmt.Sprintf("repositories/%s/%s/commit/%s/statuses/build", repo.Owner,
		repo.Name, pull.HeadCommit)

	_, err = b.do("POST", commitStatusURL, payload)
	if err != nil {
		return err
	}

	return nil
}

// GetPullRequest
func (b *BitbucketClient) GetPullRequest(repoFullName string, pullNum int) *bitbucket.PullRequests {
	return nil
}
