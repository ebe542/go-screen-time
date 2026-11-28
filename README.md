# Windows 11 Screen Time

A Windows 11 parental control project written in Go. Each child has a policy tied to their Windows account. Parents allow specific programs and websites, with optional time limits. Unrestricted school and productivity tools stay available when media time runs out.

## Planned features

- A strict allowlist of programs and websites per child, with unrestricted and time-limited entries.
- A shared daily media budget, with separate weekday and weekend settings.
- Allowed hours and individual daily limits for time-limited programs and websites.
- Required breaks after a configurable amount of use.
- Bonus minutes granted directly in the parent menu or redeemed offline from a signed, encrypted message sent by a parent.
- Separate usage history and limits for each child's Windows account.
- A password-protected parent menu for managing rules, granting time and reviewing usage.
- Enforcement of app restrictions and website restrictions in a managed browser.

The project is at the design stage. The [specification](docs/specification.md) defines proposed defaults, the Windows enforcement design and acceptance criteria.

## Requirements

- Go 1.26.6 or newer within the Go 1.26 release line
- Git
- Git Bash on Windows for the documented commands
- Administrator PowerShell for Windows service registration and file permissions
- VS Code with the official Go extension is recommended

## Configure the console service

Use the [child profile JSON reference](docs/specification.md#console-service-configuration) to create a configuration file for `screentimed -config`.

## Verify a milestone

Run the standard checks:

```console
go run ./cmd/projectctl check
```

Run the additional race detector when the local toolchain supports it:

```console
go run ./cmd/projectctl check --race
```

Validate the GitHub Actions workflow and reproduce its project checks locally:

```console
go run ./cmd/projectctl quality-gate
```

Run the complete `verify` job locally with `act` and Docker:

```console
go run ./cmd/projectctl quality-gate --act
```

## License

The project uses the [MIT License](LICENSE). The project tool includes code from Go Media Archive and retains its copyright attribution.


