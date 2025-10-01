# GitSync

## Tokens

### [GitHub](https://github.com/settings/tokens)

1. Generate new token (classic)
2. Select scopes
   - `repo`: Full control of private repositories

### [GitLab](https://gitlab.com/-/user_settings/personal_access_tokens)

1. Add new Token
2. Select scopes
   - `write_repository`: Grants read-write access to repositories on private projects using Git-over-HTTP (not using the API).
   - `api`: Grants complete read/write access to the API, including all groups and projects, the container registry, the dependency proxy, and the package registry.

### [BitBucket](https://id.atlassian.com/manage-profile/security/api-tokens)

1.  Select the **Settings** in the upper-right corner of the top navigation bar.
2.  Under **Personal settings**, select **Atlassian account settings**.
3.  Select the **Security** on the top navigation bar.
4.  Select **Create and manage API tokens**.
5.  Select **Create** **API token with scopes.**
6.  Give the API token a name and an expiry date, usually related to the application that will use the token and select **Next**.
7.  Select **Bitbucket** the app and select **Next**.
8.  Select the scopes (permissions) the API token needs and select **Next**.
    (You can search by the scope name: repo)

    - `admin:repository:bitbucket`
    - `read:repository:bitbucket`
    - `write:repository:bitbucket`

9.  Review your token and select the **Create token** button. The page will display the **New API token**.

### [Codeberg](https://codeberg.org/user/settings/applications)

1. Generate new token
2. Select permissions
   - repository: Read and write
   - user: Read and write
