# Windows 11 Screen Time

A planned Go application that helps parents manage how long children use selected Windows 11 desktop programs and games. Each child has a separate policy tied to their Windows account. When a rule blocks a program, the child can continue using the Windows desktop and other apps.

## Planned features

- A shared daily budget for selected apps, with separate weekday and weekend settings.
- Fixed hours for selected apps and individual daily limits per app or game.
- Required breaks after a configurable amount of use.
- Bonus minutes granted by a parent directly or redeemed with a one-time code.
- Separate usage history and limits for each child's Windows account.
- A password-protected parent menu for managing rules, granting time and reviewing usage.
- Automatic closure of blocked apps and enforcement on subsequent launches.

The project is at the design stage. The [specification](docs/specification.md) defines proposed defaults, the Windows enforcement design and acceptance criteria.

## Requirements

- Go 1.26.6 or newer within the Go 1.26 release line
- Git
- Git Bash on Windows for the documented commands
- VS Code with the official Go extension is recommended

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


