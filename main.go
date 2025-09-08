package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
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

var config Config
var ghClient *http.Client
var glClient *http.Client

func loadConfig() Config {
	return Config{
		GitHubUser:      mustGetEnv("GITHUB_USER"),
		GitHubToken:     mustGetEnv("GITHUB_TOKEN"),
		GitLabUser:      mustGetEnv("GITLAB_USER"),
		GitLabGroup:     getEnv("GITLAB_GROUP", ""),
		GitLabToken:     mustGetEnv("GITLAB_TOKEN"),
		RepoVisibility:  getEnv("REPO_VISIBILITY", "auto"),
		PerPage:         100,
		BackupDir:       "./repos-backup",
		LogsFolder:      "./log",
		SleepBetweenAPI: 500 * time.Millisecond,
	}
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func mustGetEnv(key string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	log.Fatalf("Environment variable %s is not set.", key)
	return ""
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
	log.SetOutput(file)
	log.SetFlags(log.LstdFlags)
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

func main() {
	config = loadConfig()
	setupLogger()
	initClients()
	log.Printf("####################### logger Started ############################")
	log.Printf("#################### Timestamp: %s ######################", time.Now().Format("2006-01-02 15:04:05"))


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
	log.Printf("✅ All Done :), Now you can enjoy hehe, please check the logs for details.")
}
