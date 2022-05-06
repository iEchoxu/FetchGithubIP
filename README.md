# FetchGithubIP

Fetch Github IP And Update Hosts

## How To Use ?
- `go mod tidy`
- `go run main.go` (Run with root privileges)

## FAQ
- Error running on windows
  - main.go add `pinger.SetPrivileged(true)`