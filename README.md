# Xylona

Xylona is a very simple control panel for running game servers on the local machine. It's designed to be 
cross-platform and easy to use.

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


4. Run the SQL migrations:
```bash
task sql-migrate-up
```

5. Start the server:
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
