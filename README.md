# Roundtable

> [!WARNING]
> Roundtable is under active development and is not production-ready.
> Use it for evaluation, testing, and experimentation only.

Roundtable is a low-latency, room-based voice chat system built with Go and WebRTC.

It includes:

- A backend signaling and room API server
- A terminal UI (TUI) client for voice chat

## Why Roundtable

Roundtable is designed to be simple to self-host and easy to extend:

- WebRTC-based audio transport
- Room lifecycle managed through a small HTTP API
- WebSocket signaling for SDP exchange and renegotiation
- SQLite (default in Docker) or PostgreSQL persistence
- Terminal-first client experience with device selection and voice activation

## Known issues / improvements

- No authentication or authorization in backend APIs.
  - Anyone who can reach the server can create/list/delete rooms and join signaling.
- No user identity or accounts yet.
  - There is currently no concept of users, profiles, or persistent user presence.
- No audible feedback for mute/unmute in the TUI.
  - Mute state is visible in UI, but there is no local earcon/notification sound.
- No room access controls.
  - Rooms are open by ID with no password, invite token, or role-based permissions.
- No indicator who is speaking.
- Limited production hardening.
  - Current defaults (for example permissive CORS) are development-friendly but not production-safe.

## Requirements

- Go 1.25+
- Docker + Docker Compose (optional, but recommended)
- Linux audio stack support for the TUI client (ALSA via malgo)

## Quick Start (Docker)

Start backend with SQLite persistence:

```bash
docker compose up --build
```

Backend will be available at:

- HTTP: `http://localhost:1323`
- WebSocket signaling: `ws://localhost:1323/ws`

## Run TUI Client

From `tui/`:

```bash
go run .
```

## Contributing

Contributions are welcome.

Suggested workflow:

1. Fork and create a feature branch.
2. Keep changes focused and documented.
3. Include or update tests where practical.
4. Open a pull request with context and validation steps.

If you plan a large change, open an issue first to discuss direction.

## License

See `LICENSE`.
