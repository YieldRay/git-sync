package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	GitHubUser      string
	GitHubToken     string
	GitLabUser      string
	GitLabGroup     string
	GitLabToken     string
	RepoVisibility  string
	PerPage         int
	BackupDir       string
	LogsFolder      string
	SleepBetweenAPI time.Duration
}

type GitHubRepo struct {
	Name     string `json:"name"`
	CloneURL string `json:"clone_url"`
	Private  bool   `json:"private"`
}

type GitLabProject struct {
	ID         int    `json:"id"`
	Visibility string `json:"visibility"`
}

var config Config
var ghClient *http.Client
var glClient *http.Client

func loadConfig() Config {
	return Config{
		GitHubUser:      getEnv("GITHUB_USER", "your-github-username"),
		GitHubToken:     getEnv("GITHUB_TOKEN", "your-github-token"),
		GitLabUser:      getEnv("GITLAB_USER", "your-gitlab-username"),
		GitLabGroup:     getEnv("GITLAB_GROUP", ""),
		GitLabToken:     getEnv("GITLAB_TOKEN", "your-gitlab-token"),
		RepoVisibility:  getEnv("REPO_VISIBILITY", "auto"),
		PerPage:         100,
		BackupDir:       "./repos-backup",
		LogsFolder:      "./logs",
		SleepBetweenAPI: 500 * time.Millisecond,
	}
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func initClients() {
	ghClient = &http.Client{}
	glClient = &http.Client{}
}

func setupLogger() {
	os.MkdirAll(config.LogsFolder, 0755)
	timestamp := time.Now().Format("20060102_150405")
	logFilePath := filepath.Join(config.LogsFolder, fmt.Sprintf("logs_%s.txt", timestamp))
	file, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Fatal(err)
	}
	log.SetOutput(io.MultiWriter(os.Stdout, file))
	log.SetFlags(log.LstdFlags)
}

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

func doGitHubRequest(method, path string, queryParams map[string]string, body io.Reader) (*http.Response, error) {
	u, err := url.Parse("https://api.github.com")
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
	req.SetBasicAuth(config.GitHubUser, config.GitHubToken)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	return ghClient.Do(req)
}

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

func handleGitHubResponse(resp *http.Response, target interface{}) error {
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
			return err
		}
		return nil
	} else {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("GitHub API error %d: %s", resp.StatusCode, string(body))
		return fmt.Errorf("GitHub API error")
	}
}

func getGitHubRepos() ([]GitHubRepo, error) {
	var repos []GitHubRepo
	page := 1
	for {
		resp, err := doGitHubRequest("GET", "/user/repos", map[string]string{
			"per_page":    strconv.Itoa(config.PerPage),
			"page":        strconv.Itoa(page),
			"affiliation": "owner,member",
		}, nil)
		if err != nil {
			return nil, err
		}
		var batch []GitHubRepo
		if err := handleGitHubResponse(resp, &batch); err != nil {
			break
		}
		if len(batch) == 0 {
			break
		}
		repos = append(repos, batch...)
		page++
		time.Sleep(config.SleepBetweenAPI)
	}
	log.Printf("Found %d GitHub repos", len(repos))
	return repos, nil
}

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

func mirrorReposFromGitHub(repoName, githubURL, localPath string) error {
	authCloneURL := strings.Replace(githubURL, "https://", fmt.Sprintf("https://%s:%s@", config.GitHubUser, config.GitHubToken), 1)
	if _, err := os.Stat(localPath); os.IsNotExist(err) {
		log.Printf("Cloning (mirror) %s ...", repoName)
		return runCmd("git", "clone", "--mirror", authCloneURL, localPath)
	} else {
		err := runCmd("git", "--git-dir", localPath, "fetch", "--all", "--prune")
		if err != nil {
			log.Printf("Recloning %s due to fetch failure", repoName)
			os.RemoveAll(localPath)
			return runCmd("git", "clone", "--mirror", authCloneURL, localPath)
		}
		return nil
	}
}

func runCmd(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
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

func main() {
	config = loadConfig()
	setupLogger()
	initClients()
	log.Printf("####################### logger Started ############################")
	log.Printf("#################### Timestamp: %s ######################", time.Now().Format("2006-01-02 15:04:05"))

	if config.GitHubToken == "your-github-token" {
		log.Fatal("GITHUB_TOKEN environment variable is not set.")
	}
	if config.GitLabToken == "your-gitlab-token" {
		log.Fatal("GITLAB_TOKEN environment variable is not set.")
	}
	if config.GitLabUser == "your-gitlab-username" {
		log.Fatal("GITLAB_USER environment variable is not set.")
	}

	repos, err := getGitHubRepos()
	if err != nil {
		log.Fatal(err)
	}
	gitlabGroupID, err := getGitLabGroupID()
	if err != nil {
		log.Fatal(err)
	}
	if len(repos) == 0 {
		log.Printf("No repos found; exiting.")
		return
	}

	os.MkdirAll(config.BackupDir, 0755)
	reposDone := 0
	for _, repo := range repos {
		repoName := repo.Name
		githubURL := repo.CloneURL
		repoVisibility := "private"
		if config.RepoVisibility == "auto" {
			if repo.Private {
				repoVisibility = "private"
			} else {
				repoVisibility = "public"
			}
		} else if config.RepoVisibility != "" {
			repoVisibility = config.RepoVisibility
		}
		localPath := filepath.Join(config.BackupDir, fmt.Sprintf("%s.git", repoName))

		log.Printf("Syncing %s", repoName)
		err := mirrorReposFromGitHub(repoName, githubURL, localPath)
		if err != nil {
			log.Printf("Failed to mirror %s: %v", repoName, err)
			continue
		}
		err = checkAndValidateGitLabRepos(gitlabGroupID, repoName, config.GitLabUser, repoVisibility)
		if err != nil {
			log.Printf("Failed to validate GitLab repo %s: %v", repoName, err)
			continue
		}
		err = syncRepos(gitlabGroupID, config.GitLabUser, config.GitLabToken, repoName, localPath)
		if err != nil {
			log.Printf("Failed to sync %s: %v", repoName, err)
			continue
		}
		log.Printf("✅ Synced %s", repoName)
		reposDone++
		log.Printf("Repos done: %d/%d", reposDone, len(repos))
	}
	log.Printf("✅ All Done :), all repositories has been synced, please check the logs for details.")
	fmt.Println("✅ All Done :), Now you can enjoy hehe, please check the logs for details.")
}
