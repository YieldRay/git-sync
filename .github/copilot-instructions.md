# GitSync AI Coding Assistant Instructions

Welcome to **GitSync**, a CLI tool for mirroring and synchronizing GitHub repositories to GitLab, Codeberg, or Bitbucket.

## 📂 Project Structure & Key Components

- **`main.go`**: Entry point. Parses `-target` flag, loads environment-based config, and drives the sync loop.
- **`github.go`**: Implements GitHub REST API client (`doGitHubRequest`, `handleGitHubResponse`, `getGitHubRepos`).
- **`gitlab.go`**: Implements GitLab REST API client, project creation/updating, and push logic.
- **`codeberg.go`**: Implements Codeberg API client and mirror-push functions.
- **`bitbucket.go`**: Implements Bitbucket v2 API client and mirror-push functions.
- **`go.mod`**: Module declaration (Go 1.25.1). No external dependencies beyond the standard library and the `git` CLI.

## 🚀 Build & Run Workflows

1. **Build**:
   ```bash
   go build -o git-sync .
   ```
2. **Run** (choose one target):
   ```bash
   # Sync to GitLab
   ./git-sync -target=gitlab

   # Sync to Codeberg
   ./git-sync -target=codeberg

   # Sync to Bitbucket
   ./git-sync -target=bitbucket
   ```
3. **Environment Variables** (required):
   - **Global**: `GITHUB_USER`, `GITHUB_TOKEN`.
   - **GitLab**: `GITLAB_USER`, `GITLAB_TOKEN`, optional `GITLAB_GROUP`.
   - **Codeberg**: `CODEBERG_USER`, `CODEBERG_TOKEN`.
   - **Bitbucket**: `BITBUCKET_USER`, `BITBUCKET_APP_PASSWORD`, optional `BITBUCKET_WORKSPACE` (defaults to `BITBUCKET_USER`).
   - **Optional**: `REPO_VISIBILITY` (`auto|public|private`, default `auto`).

## 📋 Configuration & Conventions

- **Config struct** in `main.go` centralizes all settings:
  - `PerPage`: 100 repos per API call
  - `SleepBetweenAPI`: 500ms delay between pagination
  - `BackupDir`: default `./repos-backup`
  - `LogsFolder`: default `./log`

- **Mirror cloning**:
  - Uses `git clone --mirror` into `<BackupDir>/<repo>.git`.
  - On fetch failures, it deletes and reclones.

- **HTTP patterns**:
  - `doXxxRequest(method, path, params, body)`: builds URL, sets auth headers.
  - `handleXxxResponse(resp, &struct)`: decodes JSON on 2xx, logs & errors otherwise.

- **Push logic**:
  - Constructs authenticated push URL by injecting credentials into HTTPS URL.
  - Calls `git --git-dir <path> push --mirror <pushURL>`.

## 📦 Integration & External Dependencies

- **Git CLI**: Invoked via `exec.Command` for clone, fetch, push.
- **HTTP clients**: Separate `*http.Client` for GitHub (`ghClient`) and others (`glClient`).
- **Filesystem**: Uses `os.MkdirAll` to prepare `BackupDir` and `LogsFolder`.

## 🔍 Extending the Codebase

To add a new sync target:
1. Copy one of the `*.go` clients (`github.go` / `gitlab.go`).
2. Implement `doXxxRequest`, `handleXxxResponse`, and entity struct.
3. Add `checkAndValidateXxxRepo` and `syncToXxx` functions.
4. Wire into the `main()` switch on `-target`.

## ❓ Questions & Feedback

- Are there any patterns or workflows that need more detail?
- Is any command or convention unclear? Let me know to improve this guide.