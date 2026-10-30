# Suitee Platform - Comprehensive Code Wiki

## 1. Overall Project Architecture
Suitee is a robust real estate management platform built upon a microservices architecture. It separates distinct business domains into isolated services, ensuring modularity, scalability, and maintainability.

**Key Architecture Components:**
- **Microservices (Go):** The backend consists of multiple Go (Golang 1.25+) microservices, using the Gin framework for HTTP routing. Each service manages its own domain and data in Postgres (schemas/tables with JSONB).
- **API Gateway (`gateway`):** A single entry point that reverse-proxies requests to the respective microservices based on the URL path, handles CORS, and implements basic rate limiting.
- **Frontend (`toka`):** A modern Single Page Application built with React 18, TypeScript, and Vite. It utilizes Tailwind CSS and Radix UI for its component system.
- **Databases & Cache:** Postgres (optionally Supabase Postgres) is the system of record, and Redis is used for caching and ephemeral state.
- **Event-Driven Communication:** RabbitMQ is employed to handle asynchronous inter-service communication via events (e.g., a lead converted in CRM triggers a notification in Comm).
- **Containerization:** The entire stack is orchestrated using Docker Compose, facilitating easy deployment and local development.

## 2. Responsibilities of Major Modules

### Backend Services
- **`gateway` (Port 8080):** Routes external traffic to internal microservices. Provides basic security headers, CORS configuration, and IP-based rate limiting.
- **`toca-auth` (Port 8081):** Manages Identity and Access Management (IAM). Handles user accounts, authentication (JWT), company setups, and invitations.
- **`toca-core` (Port 8082):** The central domain service for real estate. Manages `Property`, `Address`, `Partnership`, and company-wide `Settings`.
- **`toca-financial` (Port 8083):** Manages financial operations including `Transaction`, `Charge`, `Invoice`, `Commission`, `Collection`, and `Delinquency`.
- **`toca-crm` (Port 8084):** Handles Customer Relationship Management. Deals with `Lead`, broker assignments, and the lead distribution `Roulette`.
- **`toca-deal` (Port 8085):** Deal/Contract orchestrator (Saga). Tracks `Deal` (negotiations), `Contract`, contract `Template`, and emits downstream events (financial, comm, calendar, signature).
- **`toca-comm` (Port 8086):** Communication service handling emails, internal notifications, and real-time WebSocket connections.
- **`toca-ops` (Port 8087):** Operations service managing physical interactions like `Key` tracking, `Maintenance` requests, and `Inspection`.
- **`toca-calendar` (Port 8088):** Handles scheduling and `Appointment` management.
- **`toca-portal` (Port 8089):** Backend for the Client Portal experience (tenants/owners).
- **`pkg`:** A shared library package containing cross-cutting concerns like database connection utilities, centralized logging, standard HTTP responses, auth middlewares, and RabbitMQ event publishers/consumers.

### Frontend Application
- **`toka`:** The React frontend SPA. Features distinct modules for dashboards, lead tracking, property management, deal pipelines ("Esteira"), financial dashboards, and client portals.

## 3. Descriptions of Key Classes and Functions

### `toca-core` (Core Real Estate Domain)
- **Class `Property`:** The main real estate asset entity (`internal/domain/core/property/property.go`). Tracks characteristics, pricing, owners, and media.
- **Function `PropertyService.UpdateStatus`:** Validates state transitions (e.g., from `prospecting` to `available`).

### `toca-crm` (Customer Relationship Management)
- **Class `Lead`:** Represents a potential client or inquiry (`internal/domain/lead/lead.go`).
- **Class `Roulette`:** Represents the round-robin or weighted distribution mechanism for assigning new leads to brokers.
- **Function `Lead.AddHistory`:** Appends interaction history and status changes to the lead's timeline.

### `toca-deal` (Deals and Contracts)
- **Class `Deal`:** Represents a negotiation for a sale or rental (`internal/domain/deal/deal.go`). Orchestrates proposals and broker splits.
- **Class `Contract`:** Represents the legal agreement (`internal/domain/contract/contract.go`), managing signatories and status (`DRAFT`, `SIGNED`, `ACTIVE`).
- **Class `Template`:** Represents a contract template, allowing variable mapping (e.g., mapping a property's address to a document placeholder).

### `toca-auth` (Identity and Access)
- **Class `User`:** Represents a platform user (`internal/domain/user/user.go`).
- **Class `Account`:** Represents an account hierarchy, distinguishing between individual accounts and company accounts.

### `toca-financial` (Financial Operations)
- **Class `Transaction`:** Represents financial movements (revenue or expense).
- **Class `Commission`:** Tracks broker commissions tied to specific deals, including pending and paid statuses.

### `toca-portal` (External Integrations)
- **Class `Integration`:** Represents an external connection (e.g., a specific real estate portal config).
- **Class `Sync`:** Manages the state of data synchronization between Suitee and external portals.

### `pkg` (Shared Libraries)
- **Function `events.Publish` / `events.Consume`:** Standardized interfaces for interacting with RabbitMQ across all microservices.

## 4. Dependency Relationships

### Backend Dependencies (`go.mod`)
- **HTTP Routing:** `github.com/gin-gonic/gin`
- **Database (Postgres):** `github.com/jackc/pgx/v5`, `github.com/jmoiron/sqlx`
- **Caching (Redis):** `github.com/redis/go-redis/v9`
- **Message Broker (RabbitMQ):** `github.com/rabbitmq/amqp091-go`
- **Authentication:** `github.com/golang-jwt/jwt/v5`
- **Validation:** `github.com/go-playground/validator/v10`
- **API Documentation:** `github.com/swaggo/swag` and `github.com/swaggo/gin-swagger`

### Frontend Dependencies (`toka/package.json`)
- **Core:** `react`, `react-dom`, `react-router` (v7)
- **Styling:** `tailwindcss` (v4), `@radix-ui/react-*` (headless UI components), `lucide-react`
- **State & Data Fetching:** Custom hooks (e.g., `useWebSocket.ts`), standard React Contexts (`AuthContext.tsx`, `CRMContext.tsx`)
- **Document Processing:** `docxtemplater`, `pizzip`, `jspdf` for contract parsing and export.
- **Testing:** `@playwright/test` for E2E testing, `vitest` for unit testing.

## 5. Instructions for Running the Project

### Prerequisites
- Docker and Docker Compose
- Go 1.25+ (for local backend development)
- Node.js & npm (for local frontend development)
- Make (optional but highly recommended)

### Step 1: Initial Configuration
Clone the repository and set up the environment variables:
```bash
cp .env.example .env
```

### Step 2: Running the Backend Stack
The easiest way to start the infrastructure (Redis, RabbitMQ, MinIO/imgproxy and observability) and all Go microservices is via Docker Compose using the Makefile:
```bash
make run
```
*The API Gateway will now be accessible at `http://localhost:8080`.*

### Step 3: Populating Mock Data (Optional)
To seed the database with mock properties, leads, and users for development:
```bash
make dev-mock
```
*Look at the terminal output for the generated admin credentials.*

### Step 4: Running the Frontend
**Option A: Local Development (Recommended for UI work)**
```bash
cd toka
pnpm install
pnpm dev
```
*The frontend will be available at `http://localhost:5173`.*

**Option B: Dockerized Frontend**
```bash
make run-frontend
```
*The frontend will be available at `http://localhost:3000`.*

### Step 5: Stopping and Cleaning Up
To stop all running services:
```bash
make stop
```
To stop services and completely wipe all database volumes:
```bash
make reset ENV=local CONFIRM=reset
```

### Testing
To run the deterministic Playwright suite:
```bash
make test-e2e
```
To run the isolated Docker smoke followed by the deterministic Playwright
journeys:
```bash
make test-e2e-isolated
```

Esse alvo de compatibilidade executa dois gates independentes; não substitui
as specs Playwright classificadas como `fullStack`.
