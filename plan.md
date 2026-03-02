1. **Identify Vulnerability**: The `UploadAuthFile` and `DeleteAuthFile` functions in `internal/api/handlers/management/auth_files.go` have path traversal vulnerabilities because they do not properly sanitize the `name` parameter retrieved from the query string before using it to construct file paths. While `DownloadAuthFile` was previously secured with `strings.ContainsAny(name, "/\\")`, the other functions used weaker or no checks.
2. **Apply Security Fixes**:
    - Update `UploadAuthFile` to validate the `name` query parameter, ensuring it does not contain `/` or `\` characters early in the function, mimicking the robust check in `DownloadAuthFile`.
    - Update `DeleteAuthFile` to apply the same `strings.ContainsAny(name, "/\\")` check when deleting a specific file by name.
3. **Verify Security Checks**:
    - Write unit tests in `internal/api/handlers/management/auth_files_security_test.go` for `UploadAuthFile` and `DeleteAuthFile` path traversal scenarios. (Already done and passing).
4. **Pre-commit Steps**: Call `pre_commit_instructions` and follow them to ensure proper testing, verification, review, and reflection are done.
5. **Submit Change**: Submit the pull request with appropriate branch name and commit message.
