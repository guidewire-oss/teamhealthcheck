# Team Health Check Backend API

Go backend for Team Health Check Squad Health Check application, built with Gin framework following Domain-Driven Design (DDD) principles and Test-Driven Development (TDD) practices.

## Architecture

This backend follows **Domain-Driven Design (DDD)** with clear separation of concerns:

```
backend/
├── cmd/api/              # Application entry point
├── domain/               # Domain layer (business logic)
│   ├── user/            # User aggregate
│   ├── team/            # Team aggregate
│   ├── healthcheck/     # Health check aggregate
│   └── organization/    # Organization aggregate
├── application/          # Application layer (use cases)
│   ├── commands/        # Write operations
│   └── queries/         # Read operations
├── infrastructure/       # Infrastructure layer
│   ├── persistence/     # Database implementations
│   ├── http/            # HTTP clients
│   └── messaging/       # Event bus, queues
├── interfaces/          # Interface layer
│   ├── api/v1/         # API handlers (Gin)
│   ├── dto/            # Data Transfer Objects
│   └── middleware/     # Gin middleware
└── tests/              # Tests (Ginkgo/Gomega)
    └── acceptance/     # Acceptance tests
```

## Key Concepts

### Domain-Driven Design (DDD)

- **Aggregates**: `User`, `Team`, `HealthCheckSession`, `OrganizationConfig` are aggregate roots
- **Value Objects**: `HierarchyLevel`, `HealthDimension`, `HealthCheckResponse`
- **Repositories**: Abstract data access with domain-focused interfaces
- **Domain Services**: Cross-aggregate business logic
- **Domain Events**: `UserCreated`, `TeamAssigned`, `HealthCheckCompleted`, etc.

### Test-Driven Development (TDD)

We follow **outer-loop TDD** with Ginkgo:

1. Write acceptance tests first (describes user behavior)
2. Implement domain logic to make tests pass
3. Refactor with confidence

## Getting Started

### Prerequisites

- Go 1.25 or later
- Make

### Installation

```bash
# Install dependencies
make install

# This will:
# - Install Go dependencies (go mod download)
# - Install Ginkgo CLI for testing
# - Install Air for hot reloading
```

### Development

```bash
# Run the API server
make run

# Run with hot reload (recommended for development)
make dev

# API will be available at http://localhost:8080
```

### Testing

```bash
# Run all tests
make test

# Run tests with verbose output
make test-verbose

# Run tests with coverage
make test-coverage

# Run acceptance tests only
make test-acceptance

# Run tests in watch mode (auto-rerun on file changes)
make test-watch
```

### Building

```bash
# Build production binary
make build

# Binary will be created at: bin/team360-api
```

### Linting & Formatting

```bash
# Format code
make fmt

# Run go vet
make vet

# Run all linters (fmt + vet)
make lint
```

## API Endpoints

### Health Check
- `GET /health` - API health check

### Users (v1)
- `GET /api/v1/users` - List all users
- `GET /api/v1/users/:id` - Get user by ID
- `POST /api/v1/users` - Create user
- `PUT /api/v1/users/:id` - Update user
- `DELETE /api/v1/users/:id` - Delete user

### Teams (v1)
- `GET /api/v1/teams` - List all teams
- `GET /api/v1/teams/:id` - Get team by ID
- `POST /api/v1/teams` - Create team
- `PUT /api/v1/teams/:id` - Update team
- `DELETE /api/v1/teams/:id` - Delete team

### Health Checks (v1)
- `GET /api/v1/health-checks` - List health check sessions
- `GET /api/v1/health-checks/:id` - Get session by ID
- `POST /api/v1/health-checks` - Submit health check

### Organization (v1)
- `GET /api/v1/organizations/config` - Get organization config
- `PUT /api/v1/organizations/config` - Update organization config

## Environment Variables

```bash
# Server configuration
PORT=8080                 # API server port (default: 8080)
GIN_MODE=debug           # Gin mode: debug, release, test (default: debug)

# Database (to be configured)
# DB_HOST=localhost
# DB_PORT=5432
# DB_NAME=team360
# DB_USER=team360
# DB_PASSWORD=
```

## Development Workflow

### 1. Write a Test (Outer-Loop TDD)

```go
// tests/acceptance/health_check_test.go
var _ = Describe("Health Check Submission", func() {
    It("should save the session with automatic period detection", func() {
        // Given
        session := createHealthCheckSession()

        // When
        result := healthCheckService.Submit(session)

        // Then
        Expect(result.Error).To(BeNil())
        Expect(result.Session.AssessmentPeriod).To(Equal("2024 - 2nd Half"))
    })
})
```

### 2. Implement Domain Logic

```go
// domain/healthcheck/service.go
func (s *Service) Submit(session *HealthCheckSession) error {
    // Automatically determine assessment period
    session.AssessmentPeriod = getAssessmentPeriod(session.Date)

    // Save to repository
    return s.repository.Save(session)
}
```

### 3. Run Tests

```bash
make test-watch  # Tests will auto-run on file changes
```

## Technology Stack

- **Language**: Go 1.25
- **Framework**: Gin (HTTP router and middleware)
- **Architecture**: Domain-Driven Design (DDD)
- **Testing**: Ginkgo v2 (BDD framework) + Gomega (assertions)
- **Hot Reload**: Air
- **Database**: TBD (PostgreSQL recommended)
- **ORM**: TBD (GORM or sqlx recommended)

## Contributing

1. Write tests first (TDD approach)
2. Follow DDD principles (aggregate roots, value objects, repositories)
3. Ensure all tests pass: `make test`
4. Format code: `make fmt`
5. Run linters: `make lint`

## Learn More

- [Gin Framework Documentation](https://gin-gonic.com/docs/)
- [Ginkgo Testing Framework](https://onsi.github.io/ginkgo/)
- [Domain-Driven Design](https://martinfowler.com/bliki/DomainDrivenDesign.html)
