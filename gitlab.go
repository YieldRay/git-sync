// GitLab
// Personal Access Tokens: https://gitlab.com/-/user_settings/personal_access_tokens
// Projects API: https://docs.gitlab.com/ee/api/projects.html
// Groups API:   https://docs.gitlab.com/ee/api/groups.html
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
)

type GitLabProject struct {
	ID         int    `json:"id"`
	Visibility string `json:"visibility"`
}

// doGitLabRequest issues a request against the GitLab v4 API (https://gitlab.com/api/v4).
func doGitLabRequest(method, path string, queryParams map[string]string, body io.Reader) (*http.Response, error) {
	u, err := url.Parse("https://gitlab.com")
	if err != nil {
		return nil, err
	}
	u.Path = path
	q := u.Query()
	for k, v := range queryParams {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequest(method, u.String(), body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("PRIVATE-TOKEN", config.GitLabToken)
	if body != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	return glClient.Do(req)
}

// handleGitLabResponse decodes 2xx JSON responses; logs and errors otherwise.
func handleGitLabResponse(resp *http.Response, target interface{}) (interface{}, error) {
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
			return nil, err
		}
		return target, nil
	} else {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("GitLab API error %d: %s", resp.StatusCode, string(body))
		return nil, fmt.Errorf("API error")
	}
}

// Get single project
// Docs: https://docs.gitlab.com/ee/api/projects.html#get-single-project
func getGitLabProject(repoName, userName string, groupID *int) (*GitLabProject, error) {
	var projPath string
	if groupID != nil {
		projPath = fmt.Sprintf("%d/%s", *groupID, repoName)
	} else {
		projPath = fmt.Sprintf("%s/%s", userName, repoName)
	}
	encoded := url.QueryEscape(projPath)
	resp, err := doGitLabRequest("GET", fmt.Sprintf("/api/v4/projects/%s", encoded), nil, nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == 404 {
		return nil, nil // Not found
	}
	var proj GitLabProject
	result, err := handleGitLabResponse(resp, &proj)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}
	return result.(*GitLabProject), nil
}

// Details of a group
// Docs: https://docs.gitlab.com/ee/api/groups.html#details-of-a-group
func getGitLabGroupID() (*int, error) {
	if config.GitLabGroup == "" {
		return nil, nil
	}
	encoded := url.QueryEscape(config.GitLabGroup)
	resp, err := doGitLabRequest("GET", fmt.Sprintf("/api/v4/groups/%s", encoded), nil, nil)
	if err != nil {
		return nil, err
	}
	var group struct {
		ID int `json:"id"`
	}
	result, err := handleGitLabResponse(resp, &group)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, fmt.Errorf("GitLab group not found")
	}
	return &result.(*struct{ ID int }).ID, nil
}

// Edit project (update visibility)
// Docs: https://docs.gitlab.com/ee/api/projects.html#edit-project
func updateGitLabProjectVisibility(projectID int, visibility string) error {
	data := fmt.Sprintf("visibility=%s", visibility)
	resp, err := doGitLabRequest("PUT", fmt.Sprintf("/api/v4/projects/%d", projectID), nil, strings.NewReader(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 200 {
		log.Printf("Updated GitLab project %d visibility -> %s", projectID, visibility)
		return nil
	} else {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("Error updating visibility for project %d: %d %s", projectID, resp.StatusCode, string(body))
		return fmt.Errorf("failed to update visibility")
	}
}

// Create project (optionally under a group via namespace_id)
// Docs: https://docs.gitlab.com/ee/api/projects.html#create-project
func createGitLabProject(groupID *int, repoName, visibility string) (*GitLabProject, error) {
	data := fmt.Sprintf("name=%s&path=%s&visibility=%s&initialize_with_readme=false", repoName, repoName, visibility)
	if groupID != nil {
		data += fmt.Sprintf("&namespace_id=%d", *groupID)
	}
	resp, err := doGitLabRequest("POST", "/api/v4/projects", nil, strings.NewReader(data))
	if err != nil {
		return nil, err
	}
	var proj GitLabProject
	result, err := handleGitLabResponse(resp, &proj)
	if err != nil {
		return nil, err
	}
	if result != nil {
		log.Printf("Created GitLab project %s", repoName)
		return result.(*GitLabProject), nil
	}
	return nil, fmt.Errorf("unexpected response")
}

func checkAndValidateGitLabRepos(groupID *int, repoName, userName, repoVisibility string) error {
	proj, err := getGitLabProject(repoName, userName, groupID)
	if err != nil {
		return err
	}
	if proj == nil {
		log.Printf("Project %s not found on GitLab. Creating...", repoName)
		_, err = createGitLabProject(groupID, repoName, repoVisibility)
		return err
	} else {
		if proj.Visibility != repoVisibility {
			log.Printf("Project %s exists on GitLab with visibility '%s' but desired is '%s'. Updating...", repoName, proj.Visibility, repoVisibility)
			return updateGitLabProjectVisibility(proj.ID, repoVisibility)
		} else {
			log.Printf("Project %s exists on GitLab with matching visibility '%s'.", repoName, proj.Visibility)
		}
	}
	return nil
}

// https://forum.gitlab.com/t/how-to-git-clone-via-https-with-personal-access-token-in-private-project/43418
func syncRepos(groupID *int, userName, gitlabToken, repoName, localPath string) error {
	targetNamespace := config.GitLabGroup
	if groupID == nil {
		targetNamespace = userName
	}
	glRepoURL := fmt.Sprintf("https://gitlab.com/%s/%s.git", targetNamespace, repoName)
	pushURL := strings.Replace(glRepoURL, "https://", fmt.Sprintf("https://oauth2:%s@", gitlabToken), 1)
	log.Printf("Pushing %s -> GitLab (%s) ...", repoName, targetNamespace)
	return runCmd("git", "--git-dir", localPath, "push", "--mirror", pushURL)
}
