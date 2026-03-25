# Xylona

Xylona is a very simple control panel for running game servers on the local machine. It's designed to be 
cross-platform and easy to use.

## Warning
This project is in its early stages and is not yet ready for production use. Use at your own risk.

## Features

- Start, stop, and restart game servers.
- View server logs.
- Configure server settings.
- Manage server files.
- View server status.


## Using Xylona

1. Download the latest release from the release page.


## Development

These instructions will get you a copy of the project up and running on your local machine for development and testing purposes.

### Prerequisites

- [Go 1.22+](https://go.dev/doc/install)
- [pnpm](https://pnpm.io/installation)
- [Taskfile](https://taskfile.dev/installation/)

### Backend

1. Clone the repository:
```bash
git clone https://github.com/ClintonCollins/Xylona.git
```

2. Navigate to the project directory:
```bash
cd Xylona
```

3. Configure `sql/dbconfig.yml` with your database credentials.

4. Set your signing and encryption secrets in the environment or `.env`:
```bash
JWT_SECRET_KEY_BASE64=<base64-encoded 32+ byte signing key>
ENCRYPTION_KEY_BASE64=<base64-encoded 32+ byte encryption key>
```
`ENCRYPTION_KEY_BASE64` is strongly recommended. If it is omitted, Xylona falls back to the JWT secret for DB encryption and will log a warning at startup.

5. Run the SQL migrations:
```bash
task sql-migrate-up
```

6. Start the server:
```bash
go build -o xylona && ./xylona
```

The server will start on `localhost:8080`.

### Frontend

1. Navigate to the `frontend` directory:
```bash
cd frontend
```
2. Install the dependencies:
```bash
pnpm install
```
3. Start the development server:
```bash
pnpm run dev
```
The development server will start on `localhost:3000`.

### E2E Testing

#### Single-Node Tests
Requires a running backend on `:8080`:
```bash
pnpm --dir frontend run e2e
# Or: mage E2E
```

#### Federation Tests
Fully self-contained — builds binaries, starts two Xylona nodes, pairs them, and runs tests:
```bash
pnpm --dir frontend run e2e:federation
# Or: mage E2EFederation
```

#### Debugging
```bash
# Run in headed mode to watch the browser
mage E2EHeaded
mage E2EFederationHeaded

# Keep federation data after test run
E2E_KEEP_DATA=1 pnpm --dir frontend run e2e:federation

# View HTML reports
mage E2EReport
mage E2EFederationReport
```

#### E2E Orchestrator
All setup and teardown logic lives in a Go CLI tool (`cmd/e2e`). The TypeScript Playwright configs call it automatically, but you can run it directly:
```bash
# Seed a fresh database with an admin user
go run ./cmd/e2e seed -db <path> -username admin -password admin

# Run single-node setup/teardown manually
go run ./cmd/e2e single-setup --backend-url http://localhost:8080
go run ./cmd/e2e single-teardown --backend-url http://localhost:8080
```

### Recommendations

It's suggested to install Docker and Docker Compose to run Caddy in a container. This will make it easy to proxy
requests between the backend and frontend.

1. Navigate to `docker` directory:
```bash
cd xylona/docker
```
2. Start the Caddy container:
```bash
docker-compose up -d
```
3. Access the frontend at `https://localhost`
