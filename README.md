# Caddyshack

A web-based management UI for [Caddy](https://caddyserver.com/) server.

Caddyshack provides a friendly interface for managing your Caddy reverse proxy configuration, eliminating the need to manually edit Caddyfiles.

## Features

- Dashboard showing all configured sites
- Add, edit, and delete site configurations
- Support for common patterns: reverse proxy, static files, redirects
- Caddyfile syntax validation before saving
- Automatic Caddy reload after changes (via Admin API)
- Configuration history with rollback support
- Basic auth protection for the UI

## Tech Stack

- **Backend**: Go (standard library + chi router)
- **Frontend**: Go html/template + HTMX + Alpine.js + Tailwind CSS
- **Database**: SQLite (pure Go driver, no CGO required)
- **Deployment**: Single binary in Docker

## Installation

### Prerequisites

- Go 1.21 or later
- Node.js (for Tailwind CSS build)
- Docker (for containerized deployment)

### From Source

```bash
# Clone the repository
git clone https://github.com/dustinredseam/caddyshack.git
cd caddyshack

# Build
go build -o caddyshack ./cmd/caddyshack

# Run
./caddyshack
```

### Docker

```bash
# Build the image
docker build -t caddyshack .

# Run
docker run -p 8080:8080 \
  -v /path/to/Caddyfile:/etc/caddy/Caddyfile \
  caddyshack
```

## Configuration

Caddyshack is configured via environment variables:

| Variable               | Description                 | Default                 |
| ---------------------- | --------------------------- | ----------------------- |
| `CADDYSHACK_PORT`      | Port to listen on           | `8080`                  |
| `CADDYSHACK_CADDYFILE` | Path to Caddyfile to manage | `/etc/caddy/Caddyfile`  |
| `CADDYSHACK_CADDY_API` | Caddy Admin API URL         | `http://localhost:2019` |
| `CADDYSHACK_DB`        | SQLite database path        | `./caddyshack.db`       |
| `CADDYSHACK_AUTH_USER` | Basic auth username         | `admin`                 |
| `CADDYSHACK_AUTH_PASS` | Basic auth password         | `changeme`              |

## Development

```bash
# Run development server
go run ./cmd/caddyshack

# Run tests
go test ./...

# Build Tailwind CSS (after modifying styles)
npx tailwindcss -i ./static/css/input.css -o ./static/css/output.css
```

## License

MIT License - see [LICENSE](LICENSE) for details.
