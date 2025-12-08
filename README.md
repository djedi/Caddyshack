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
git clone https://github.com/djedi/caddyshack.git
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

## Development

```bash
# Run development server (with hot-reloading for templates/static)
CADDYSHACK_DEV=1 go run ./cmd/caddyshack

# Run tests
go test ./...

# Build Tailwind CSS (after modifying styles)
npx tailwindcss -i ./static/css/input.css -o ./static/css/output.css

# Watch for Tailwind changes during development
npm run watch
```

## Deployment

### Production Build

Templates and static assets are embedded into the binary, making deployment simple:

```bash
# Build production Tailwind CSS (minified)
npm run build

# Build Go binary (templates and static files are embedded)
go build -ldflags="-w -s" -o caddyshack ./cmd/caddyshack

# The resulting binary is self-contained - no external files needed
./caddyshack
```

### Docker Deployment (Recommended)

```bash
# Build the image
docker build -t caddyshack .

# Run with docker-compose (includes Caddy)
docker-compose -f docker-compose.dev.yml up
```

Docker Compose example for production:

```yaml
version: "3.8"
services:
  caddyshack:
    image: caddyshack
    ports:
      - "8080:8080"
    environment:
      - CADDYSHACK_CADDYFILE=/etc/caddy/Caddyfile
      - CADDYSHACK_CADDY_API=http://caddy:2019
      - CADDYSHACK_DB=/data/caddyshack.db
      - CADDYSHACK_AUTH_USER=admin
      - CADDYSHACK_AUTH_PASS=your-secure-password
    volumes:
      - ./Caddyfile:/etc/caddy/Caddyfile
      - caddyshack-data:/data

  caddy:
    image: caddy:2-alpine
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - ./Caddyfile:/etc/caddy/Caddyfile
      - caddy-data:/data

volumes:
  caddyshack-data:
  caddy-data:
```

### Health Check

The `/health` endpoint returns `200 OK` and can be used for load balancer health checks:

```bash
curl http://localhost:8080/health
# Returns: ok
```

For comprehensive health status with component details, use the `/health/full` endpoint:

```bash
curl http://localhost:8080/health/full
```

Returns JSON with status of all components:

```json
{
  "status": "healthy",
  "timestamp": "2024-01-15T10:30:00Z",
  "components": {
    "database": {
      "status": "healthy",
      "message": "connected",
      "latency": "1.234ms"
    },
    "caddy": {
      "status": "healthy",
      "message": "running (Caddy/2.7.0)",
      "latency": "5.678ms"
    },
    "docker": {
      "status": "healthy",
      "message": "connected",
      "latency": "2.345ms"
    }
  }
}
```

Status values:
- `healthy` - All critical components operational
- `degraded` - Non-critical components (Docker) unavailable
- `unhealthy` - Critical components (database, Caddy) unavailable (returns HTTP 503)

### Environment Variables

| Variable                 | Description                              | Default                 |
| ------------------------ | ---------------------------------------- | ----------------------- |
| `CADDYSHACK_PORT`        | Port to listen on                        | `8080`                  |
| `CADDYSHACK_DEV`         | Enable dev mode (filesystem templates)   | `false`                 |
| `CADDYSHACK_CADDYFILE`   | Path to Caddyfile to manage              | `/etc/caddy/Caddyfile`  |
| `CADDYSHACK_CADDY_API`   | Caddy Admin API URL                      | `http://localhost:2019` |
| `CADDYSHACK_DB`          | SQLite database path                     | `./caddyshack.db`       |
| `CADDYSHACK_AUTH_USER`   | Auth username                            | (disabled if not set)   |
| `CADDYSHACK_AUTH_PASS`   | Auth password                            | (disabled if not set)   |
| `CADDYSHACK_HISTORY_LIMIT` | Max config history entries             | `50`                    |
| `CADDYSHACK_DOCKER_ENABLED` | Enable Docker container integration   | `false`                 |
| `CADDYSHACK_DOCKER_SOCKET` | Path to Docker socket                  | `/var/run/docker.sock`  |

### Docker Container Integration

Caddyshack can display the status of Docker containers associated with your reverse proxy targets. This helps you see at a glance if a backend service is running.

To enable this feature:

1. Set `CADDYSHACK_DOCKER_ENABLED=true`
2. Mount the Docker socket into the container

```yaml
services:
  caddyshack:
    image: xhenxhe/caddyshack
    environment:
      - CADDYSHACK_DOCKER_ENABLED=true
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      # ... other volumes
```

**Security note:** Mounting the Docker socket gives Caddyshack read access to your Docker daemon. It can see all containers, their configurations, and environment variables. This is a common pattern for Docker management tools but be aware of the implications in multi-tenant environments.

## License

MIT License - see [LICENSE](LICENSE) for details.
