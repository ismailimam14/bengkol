# Bengkol — Workshop & Repair-Shop Management Platform

A high-performance auto repair-shop / workshop management system connecting vehicle owners with nearby service providers. Features slot reservations, real-time queues, service catalogs, inventory management, and verified service reviews.

---

## Architecture Overview

```
                      +-----------------------------+
                      |      Go REST API Backend    |
                      |   (Chi / sqlc / PostGIS)    |
                      +--------------+--------------+
                                     |
               +---------------------+---------------------+
               |                                           |
    +----------v----------+                     +----------v----------+
    |   Customer Android  |                     |    Owner Android    |
    |  (Compose / MVVM)   |                     |  (Compose / MVVM)   |
    +---------------------+                     +---------------------+
               |                                           |
               +---------------------+---------------------+
                                     |
                      +--------------v--------------+
                      |     PostgreSQL + PostGIS    |
                      +-----------------------------+
```

---

## Features & Implemented Backend Modules

1. **Authentication & RBAC (Phase 2)**:
   - Secure Argon2/bcrypt password hashing, JWT Access & Refresh Token rotation.
   - Strict Role-Based Access Control (`CUSTOMER`, `OWNER`, `ADMIN`).
   - Profile & session management (`GET /api/v1/auth/me`).

2. **Workshop Discovery & Spatial Search (Phase 3)**:
   - PostGIS spatial indexing (`ST_DWithin`, `ST_Distance`).
   - Nearby workshop discovery with radius filtering and calculated geodesic distance in km/meters.
   - Workshop profile management and operating hours configuration.

3. **Services & Spare Parts Catalog (Phase 4)**:
   - Service catalog management (pricing, estimated service duration in minutes).
   - Spare parts catalog with live stock tracking and low-stock indicators.
   - Strict workshop ownership verification.

4. **Operating Hours, Booking Slots & Concurrency (Phase 5)**:
   - Dynamic 1-hour interval booking slot computation from operating hours.
   - High-concurrency row locking (`SELECT ... FOR UPDATE`) to prevent double-booking.
   - Booking cancellations with automated slot capacity restoration.

5. **Queue Management & Live Wait Time Estimation (Phase 6)**:
   - Per-workshop daily queue sequence numbering (`1, 2, 3...`).
   - Complete lifecycle status machine: `WAITING` -> `CALLED` -> `IN_SERVICE` -> `COMPLETED` / `NO_SHOW` / `CANCELLED`.
   - Real-time wait time estimation (`customers_ahead * 20 min`).
   - Public queue summary dashboard for workshops.

6. **Service History & Spare Parts Billing (Phase 7)**:
   - Historical service records with price snapshots for services and used spare parts.
   - Automatic spare parts stock decrements upon service completion.
   - Customer service log history and workshop audit trail.

7. **Reviews & Workshop Ratings (Phase 8)**:
   - 1 to 5 star customer reviews strictly verified for completed bookings.
   - Automatic incremental recalculation of workshop average rating and total review counts.
   - Paginated review listings with customer profiles.

8. **Real-Time Communication & WebSocket Engine (Phase 9)**:
   - Full-duplex WebSocket connections (`/api/v1/ws`) with JWT handshake authentication.
   - Multiplexed channel and topic routing (`workshop:{id}`, `user:{id}`, `booking:{id}`).
   - Instantaneous event broadcasting for live queue calling, status updates, and bookings.

9. **Push Notifications & FCM Dispatcher (Phase 10)**:
   - Device token registration (`POST /api/v1/devices`, `DELETE /api/v1/devices/{token}`, `GET /api/v1/me/devices`).
   - Push notification dispatcher (`FCMDispatcher`) triggering native OS alerts when queues are called, service starts/completes, or bookings update.
   - Dual delivery mechanism: real-time WebSocket when app is foregrounded + FCM background push when app is closed.

---

## Local Development Quickstart

### Prerequisites
- Docker & Docker Compose
- Go 1.22+ (optional if running via Docker)

### 1. Start Infrastructure & Backend (Docker Compose)
```bash
docker compose up -d
```
This spins up:
- PostgreSQL 16 with PostGIS (`postgis/postgis:16-3.4-alpine`) on port `5432`
- Bengkol Go Backend on port `8080` (auto-applies migrations on boot)

### 2. Run Backend Standalone (Direct Go)
```bash
cd backend
cp .env.example .env

# Run database via docker
docker compose up -d postgres

# Run backend
go run ./cmd/api
```

### 3. Run Unit Tests
```bash
cd backend
go test -v ./...
```

---

## Interactive API Documentation & Verification Endpoints

- **Swagger UI Interactive Explorer**: `http://localhost:8080/docs` (or `http://localhost:8080/swagger`)
- **ReDoc Interactive Documentation**: `http://localhost:8080/redoc`
- **OpenAPI 3.0 YAML Specification**: `http://localhost:8080/docs/openapi.yaml`
- **Liveness Health Check**: `GET http://localhost:8080/health`
- **Readiness Health Check**: `GET http://localhost:8080/ready`
- **API Ping**: `GET http://localhost:8080/api/v1/ping`
- **Real-time WebSocket Endpoint**: `ws://localhost:8080/api/v1/ws?token=<jwt_access_token>`

---

## Environment Variables

| Variable | Default | Description |
|---|---|---|
| `APP_ENV` | `development` | Runtime environment (`development`, `production`) |
| `APP_PORT` | `8080` | Port for the HTTP server |
| `APP_NAME` | `bengkol-api` | Application service name |
| `LOG_LEVEL` | `debug` | Log severity (`debug`, `info`, `warn`, `error`) |
| `DB_HOST` | `localhost` | PostgreSQL host |
| `DB_PORT` | `5432` | PostgreSQL port |
| `DB_USER` | `postgres` | PostgreSQL username |
| `DB_PASSWORD` | `postgrespassword` | PostgreSQL password |
| `DB_NAME` | `bengkol_db` | Database name |
| `DB_SSL_MODE` | `disable` | SSL mode (`disable`, `require`) |
| `JWT_ACCESS_SECRET` | - | Secret key for signing access tokens |
| `JWT_REFRESH_SECRET` | - | Secret key for signing refresh tokens |
| `JWT_ACCESS_EXPIRY_MINUTES` | `15` | Access token lifespan in minutes |
| `JWT_REFRESH_EXPIRY_DAYS` | `7` | Refresh token lifespan in days |
