# Claude Code Configuration - SPARC Development Environment

## 🚨 CRITICAL: CONCURRENT EXECUTION & FILE MANAGEMENT

**ABSOLUTE RULES**:
1. ALL operations MUST be concurrent/parallel in a single message
2. **NEVER save working files, text/mds and tests to the root folder**
3. ALWAYS organize files in appropriate subdirectories
4. **USE CLAUDE CODE'S TASK TOOL** for spawning agents concurrently, not just MCP

### ⚡ GOLDEN RULE: "1 MESSAGE = ALL RELATED OPERATIONS"

**MANDATORY PATTERNS:**
- **TodoWrite**: ALWAYS batch ALL todos in ONE call (5-10+ todos minimum)
- **Task tool (Claude Code)**: ALWAYS spawn ALL agents in ONE message with full instructions
- **File operations**: ALWAYS batch ALL reads/writes/edits in ONE message
- **Bash commands**: ALWAYS batch ALL terminal operations in ONE message
- **Memory operations**: ALWAYS batch ALL memory store/retrieve in ONE message

### 🎯 CRITICAL: Claude Code Task Tool for Agent Execution

**Claude Code's Task tool is the PRIMARY way to spawn agents:**
```javascript
// ✅ CORRECT: Use Claude Code's Task tool for parallel agent execution
[Single Message]:
  Task("Research agent", "Analyze requirements and patterns...", "researcher")
  Task("Coder agent", "Implement core features...", "coder")
  Task("Tester agent", "Create comprehensive tests...", "tester")
  Task("Reviewer agent", "Review code quality...", "reviewer")
  Task("Architect agent", "Design system architecture...", "system-architect")
```

**MCP tools are ONLY for coordination setup:**
- `mcp__claude-flow__swarm_init` - Initialize coordination topology
- `mcp__claude-flow__agent_spawn` - Define agent types for coordination
- `mcp__claude-flow__task_orchestrate` - Orchestrate high-level workflows

### 📁 File Organization Rules

**For project source files, use these directories:**
- `/frontend` - Next.js frontend application
- `/backend` - Go backend application
- `/tests` - End-to-end acceptance tests (spans both frontend and backend)
- `/docs` - Additional documentation (beyond CLAUDE.md)
- `/scripts` - Utility scripts

**CLAUDE.md is an exception** - It lives in the root and contains project documentation for Claude Code.

**Important**: E2E acceptance tests live in `/tests/acceptance/` (not `/backend/tests/acceptance/`) because they test the complete application stack (frontend + backend + database), not just the backend. Backend unit and integration tests remain in `/backend/` subdirectories.

## SPARC Commands

### Core Commands
- `claude-flow sparc modes` - List available modes
- `claude-flow sparc run <mode> "<task>"` - Execute specific mode
- `claude-flow sparc tdd "<feature>"` - Run complete TDD workflow
- `claude-flow sparc info <mode>` - Get mode details

### Batchtools Commands
- `claude-flow sparc batch <modes> "<task>"` - Parallel execution
- `claude-flow sparc pipeline "<task>"` - Full pipeline processing
- `claude-flow sparc concurrent <mode> "<tasks-file>"` - Multi-task processing

## SPARC Workflow Phases

1. **Specification** - Requirements analysis (`sparc run spec-pseudocode`)
2. **Pseudocode** - Algorithm design (`sparc run spec-pseudocode`)
3. **Architecture** - System design (`sparc run architect`)
4. **Refinement** - TDD implementation (`sparc tdd`)
5. **Completion** - Integration (`sparc run integration`)

## 🚀 Available Agents (54 Total)

### Core Development
`coder`, `reviewer`, `tester`, `planner`, `researcher`

### Swarm Coordination
`hierarchical-coordinator`, `mesh-coordinator`, `adaptive-coordinator`, `collective-intelligence-coordinator`, `swarm-memory-manager`

### Consensus & Distributed
`byzantine-coordinator`, `raft-manager`, `gossip-coordinator`, `consensus-builder`, `crdt-synchronizer`, `quorum-manager`, `security-manager`

### Performance & Optimization
`perf-analyzer`, `performance-benchmarker`, `task-orchestrator`, `memory-coordinator`, `smart-agent`

### GitHub & Repository
`github-modes`, `pr-manager`, `code-review-swarm`, `issue-tracker`, `release-manager`, `workflow-automation`, `project-board-sync`, `repo-architect`, `multi-repo-swarm`

### SPARC Methodology
`sparc-coord`, `sparc-coder`, `specification`, `pseudocode`, `architecture`, `refinement`

### Specialized Development
`backend-dev`, `mobile-dev`, `ml-developer`, `cicd-engineer`, `api-docs`, `system-architect`, `code-analyzer`, `base-template-generator`

### Testing & Validation
`tdd-london-swarm`, `production-validator`

### Migration & Planning
`migration-planner`, `swarm-init`

## 🎯 Claude Code vs MCP Tools

### Claude Code Handles ALL EXECUTION:
- **Task tool**: Spawn and run agents concurrently for actual work
- File operations (Read, Write, Edit, MultiEdit, Glob, Grep)
- Code generation and programming
- Bash commands and system operations
- Implementation work
- Project navigation and analysis
- TodoWrite and task management
- Git operations
- Package management
- Testing and debugging

### MCP Tools ONLY COORDINATE:
- Swarm initialization (topology setup)
- Agent type definitions (coordination patterns)
- Task orchestration (high-level planning)
- Memory management
- Neural features
- Performance tracking
- GitHub integration

**KEY**: MCP coordinates the strategy, Claude Code's Task tool executes with real agents.

## 🚀 Quick Setup

```bash
# Add MCP servers (Claude Flow required, others optional)
# Use locally installed claude-flow (not npx) to avoid cache conflicts
claude mcp add claude-flow claude-flow mcp start
claude mcp add ruv-swarm ruv-swarm mcp start  # Optional: Enhanced coordination
claude mcp add flow-nexus flow-nexus mcp start  # Optional: Cloud features
```

## MCP Tool Categories

### Coordination
`swarm_init`, `agent_spawn`, `task_orchestrate`

### Monitoring
`swarm_status`, `agent_list`, `agent_metrics`, `task_status`, `task_results`

### Memory & Neural
`memory_usage`, `neural_status`, `neural_train`, `neural_patterns`

### GitHub Integration
`github_swarm`, `repo_analyze`, `pr_enhance`, `issue_triage`, `code_review`

### System
`benchmark_run`, `features_detect`, `swarm_monitor`

### Flow-Nexus MCP Tools (Optional Advanced Features)
Flow-Nexus extends MCP capabilities with 70+ cloud-based orchestration tools:

**Key MCP Tool Categories:**
- **Swarm & Agents**: `swarm_init`, `swarm_scale`, `agent_spawn`, `task_orchestrate`
- **Sandboxes**: `sandbox_create`, `sandbox_execute`, `sandbox_upload` (cloud execution)
- **Templates**: `template_list`, `template_deploy` (pre-built project templates)
- **Neural AI**: `neural_train`, `neural_patterns`, `seraphina_chat` (AI assistant)
- **GitHub**: `github_repo_analyze`, `github_pr_manage` (repository management)
- **Real-time**: `execution_stream_subscribe`, `realtime_subscribe` (live monitoring)
- **Storage**: `storage_upload`, `storage_list` (cloud file management)

**Authentication Required:**
- Register: `mcp__flow-nexus__user_register` or `npx flow-nexus@latest register`
- Login: `mcp__flow-nexus__user_login` or `npx flow-nexus@latest login`
- Access 70+ specialized MCP tools for advanced orchestration

## 🚀 Agent Execution Flow with Claude Code

### The Correct Pattern:

1. **Optional**: Use MCP tools to set up coordination topology
2. **REQUIRED**: Use Claude Code's Task tool to spawn agents that do actual work
3. **REQUIRED**: Each agent runs hooks for coordination
4. **REQUIRED**: Batch all operations in single messages

### Example Full-Stack Development:

```javascript
// Single message with all agent spawning via Claude Code's Task tool
[Parallel Agent Execution]:
  Task("Backend Developer", "Build REST API with Express. Use hooks for coordination.", "backend-dev")
  Task("Frontend Developer", "Create React UI. Coordinate with backend via memory.", "coder")
  Task("Database Architect", "Design PostgreSQL schema. Store schema in memory.", "code-analyzer")
  Task("Test Engineer", "Write Jest tests. Check memory for API contracts.", "tester")
  Task("DevOps Engineer", "Setup Docker and CI/CD. Document in memory.", "cicd-engineer")
  Task("Security Auditor", "Review authentication. Report findings via hooks.", "reviewer")

  // All todos batched together
  TodoWrite { todos: [...8-10 todos...] }

  // All file operations together
  Write "backend/server.js"
  Write "frontend/App.jsx"
  Write "database/schema.sql"
```

## 📋 Agent Coordination Protocol

### Every Agent Spawned via Task Tool MUST:

**1️⃣ BEFORE Work:**
```bash
claude-flow hooks pre-task --description "[task]"
claude-flow hooks session-restore --session-id "swarm-[id]"
```

**2️⃣ DURING Work:**
```bash
claude-flow hooks post-edit --file "[file]" --memory-key "swarm/[agent]/[step]"
claude-flow hooks notify --message "[what was done]"
```

**3️⃣ AFTER Work:**
```bash
claude-flow hooks post-task --task-id "[task]"
claude-flow hooks session-end --export-metrics true
```

## 🎯 Concurrent Execution Examples

### ✅ CORRECT WORKFLOW: MCP Coordinates, Claude Code Executes

```javascript
// Step 1: MCP tools set up coordination (optional, for complex tasks)
[Single Message - Coordination Setup]:
  mcp__claude-flow__swarm_init { topology: "mesh", maxAgents: 6 }
  mcp__claude-flow__agent_spawn { type: "researcher" }
  mcp__claude-flow__agent_spawn { type: "coder" }
  mcp__claude-flow__agent_spawn { type: "tester" }

// Step 2: Claude Code Task tool spawns ACTUAL agents that do the work
[Single Message - Parallel Agent Execution]:
  // Claude Code's Task tool spawns real agents concurrently
  Task("Research agent", "Analyze API requirements and best practices. Check memory for prior decisions.", "researcher")
  Task("Coder agent", "Implement REST endpoints with authentication. Coordinate via hooks.", "coder")
  Task("Database agent", "Design and implement database schema. Store decisions in memory.", "code-analyzer")
  Task("Tester agent", "Create comprehensive test suite with 90% coverage.", "tester")
  Task("Reviewer agent", "Review code quality and security. Document findings.", "reviewer")

  // Batch ALL todos in ONE call
  TodoWrite { todos: [
    {id: "1", content: "Research API patterns", status: "in_progress", priority: "high"},
    {id: "2", content: "Design database schema", status: "in_progress", priority: "high"},
    {id: "3", content: "Implement authentication", status: "pending", priority: "high"},
    {id: "4", content: "Build REST endpoints", status: "pending", priority: "high"},
    {id: "5", content: "Write unit tests", status: "pending", priority: "medium"},
    {id: "6", content: "Integration tests", status: "pending", priority: "medium"},
    {id: "7", content: "API documentation", status: "pending", priority: "low"},
    {id: "8", content: "Performance optimization", status: "pending", priority: "low"}
  ]}

  // Parallel file operations
  Bash "mkdir -p app/{src,tests,docs,config}"
  Write "app/package.json"
  Write "app/src/server.js"
  Write "app/tests/server.test.js"
  Write "app/docs/API.md"
```

### ❌ WRONG (Multiple Messages):
```javascript
Message 1: mcp__claude-flow__swarm_init
Message 2: Task("agent 1")
Message 3: TodoWrite { todos: [single todo] }
Message 4: Write "file.js"
// This breaks parallel coordination!
```

## Performance Benefits

- **84.8% SWE-Bench solve rate**
- **32.3% token reduction**
- **2.8-4.4x speed improvement**
- **27+ neural models**

## Hooks Integration

### Pre-Operation
- Auto-assign agents by file type
- Validate commands for safety
- Prepare resources automatically
- Optimize topology by complexity
- Cache searches

### Post-Operation
- Auto-format code
- Train neural patterns
- Update memory
- Analyze performance
- Track token usage

### Session Management
- Generate summaries
- Persist state
- Track metrics
- Restore context
- Export workflows

## Advanced Features (v2.0.0)

- 🚀 Automatic Topology Selection
- ⚡ Parallel Execution (2.8-4.4x speed)
- 🧠 Neural Training
- 📊 Bottleneck Analysis
- 🤖 Smart Auto-Spawning
- 🛡️ Self-Healing Workflows
- 💾 Cross-Session Memory
- 🔗 GitHub Integration

## Integration Tips

1. Start with basic swarm init
2. Scale agents gradually
3. Use memory for context
4. Monitor progress regularly
5. Train patterns from success
6. Enable hooks automation
7. Use GitHub tools first

## Support

- Documentation: https://github.com/ruvnet/claude-flow
- Issues: https://github.com/ruvnet/claude-flow/issues
- Flow-Nexus Platform: https://flow-nexus.ruv.io (registration required for cloud features)

---

Remember: **Claude Flow coordinates, Claude Code creates!**

---

# ═══════════════════════════════════════════════════════════════════════════════
# PROJECT DOCUMENTATION: Team360 Health Check Application
# ═══════════════════════════════════════════════════════════════════════════════

## Project Overview

Team360 is an open-source (Apache 2.0 licensed) web application based on Spotify's Squad Health Check Model, enhanced with organizational flexibility and hierarchical management capabilities.

**Architecture**: Next.js 15 frontend with Go backend (Gin framework), following Domain-Driven Design (DDD) principles and Test-Driven Development (TDD) practices.

### Purpose & Philosophy

Inspired by [Spotify's Squad Health Check Model](https://engineering.atspotify.com/2014/09/squad-health-check-model), Team360 helps teams systematically assess their working environment and identify improvement areas through structured self-reflection.

**Core Philosophy** (from Spotify):
- Teams are intrinsically motivated and want to succeed
- Health checks are a **support tool**, not a performance evaluation mechanism
- "Red" doesn't mean "bad team" - it means "this team needs support in this area"
- Focus on building team self-awareness and driving targeted improvements
- High-trust environment where honest assessment is encouraged

**Problem It Solves**: Organizational improvement is often a guessing game. Teams lack visibility into what needs fixing and whether changes actually help. Health checks provide structured visualization to reduce uncertainty across multiple perspectives (quality, value, speed, learning, fun, etc.).

### Enhancements Over Original Spotify Model

Team360 extends the original concept with enterprise-ready features:

1. **Hierarchical Organization Support**: Multi-level hierarchy (VP → Director → Manager → Team Lead → Team Member) with supervisor chains
2. **Configurable Dimensions**: 11 health dimensions (expanded from Spotify's ~8), fully customizable
3. **Flexible Cadences**: Weekly, biweekly, monthly, or quarterly check-ins per team
4. **Assessment Periods**: Track trends across defined periods (e.g., "2024 - 1st Half")
5. **Role-Based Access Control**: Granular permissions system for different organizational levels
6. **Aggregated Insights**: Roll-up views for managers and executives across multiple teams
7. **Digital-First**: While Spotify emphasizes face-to-face workshops, Team360 enables distributed teams to participate asynchronously

Built with Next.js 15, TypeScript, and Tailwind CSS for modern, maintainable development.

## Backend Architecture

### Technology Stack

- **Language**: Go 1.25+ (use latest stable as of October 2025)
- **Framework**: Gin (latest version - Go web framework)
- **Architecture**: Domain-Driven Design (DDD)
- **Testing**: Test-Driven Development (TDD) with outer-loop testing
  - **Test Framework**: Ginkgo v2 (latest - BDD-style testing framework)
  - **Assertion Library**: Gomega (latest - matcher/assertion library)
- **Dependencies**: Always use latest stable versions as of October 2025

### Domain-Driven Design Structure

The Go backend follows DDD principles with clear separation of concerns:

```
backend/
├── domain/           # Domain layer (entities, value objects, domain services)
│   ├── user/
│   ├── team/
│   ├── healthcheck/
│   └── organization/
├── application/      # Application layer (use cases, application services)
│   ├── commands/     # Command handlers (write operations)
│   └── queries/      # Query handlers (read operations)
├── infrastructure/   # Infrastructure layer (repositories, external services)
│   ├── persistence/  # Database implementations
│   ├── http/         # HTTP clients, external APIs
│   └── messaging/    # Event bus, message queues
├── interfaces/       # Interface layer (API controllers, DTOs)
│   ├── api/          # Gin HTTP handlers
│   │   └── v1/       # API version 1
│   ├── dto/          # Data Transfer Objects
│   └── middleware/   # Gin middleware
└── tests/            # Integration and acceptance tests
    └── acceptance/   # Ginkgo acceptance tests
```

**Key DDD Concepts**:
- **Aggregates**: User, Team, HealthCheckSession, Organization are aggregate roots
- **Value Objects**: HierarchyLevel, HealthDimension, HealthCheckResponse
- **Domain Events**: UserCreated, TeamAssigned, HealthCheckCompleted, etc.
- **Repositories**: Abstract data access with domain-focused interfaces
- **Domain Services**: Cross-aggregate business logic

### Test-Driven Development with Ginkgo/Gomega

**Outer-Loop TDD Approach**:
1. Write acceptance tests first using Ginkgo (describes user behavior)
2. Implement domain logic to make tests pass
3. Refactor with confidence

**Ginkgo Test Structure**:
```go
var _ = Describe("Health Check Submission", func() {
    Context("when a team member submits a health check", func() {
        It("should save the session and update team metrics", func() {
            // Given
            session := createHealthCheckSession()

            // When
            err := healthCheckService.Submit(session)

            // Then
            Expect(err).NotTo(HaveOccurred())
            Expect(session.Completed).To(BeTrue())
        })
    })
})
```

**Testing Layers**:
- **Unit Tests**: Domain logic, value objects (within `backend/domain/` packages)
- **Integration Tests**: Repository implementations, API handlers (within `backend/tests/integration/`)
- **E2E Acceptance Tests**: Complete user scenarios spanning frontend + backend + database (in `/tests/acceptance/` at repository root)

**E2E Test Architecture**:
- Located at `/tests/acceptance/` (repository root, not in backend)
- Has its own Go module (`/tests/go.mod`) with Ginkgo, Gomega, Playwright dependencies
- Tests the complete application stack: Next.js frontend → Gin backend → PostgreSQL database
- Uses Playwright for browser automation to simulate real user interactions
- Verifies data persistence in PostgreSQL to ensure complete integration
- Run from the `tests/` directory, not from `backend/`

### Backend Commands

```bash
# Navigate to backend directory
cd backend

# Install dependencies
go mod download

# Run all tests
go test ./...

# Run Ginkgo tests (with verbose output)
ginkgo -v ./...

# Run tests with coverage
ginkgo -cover ./...

# Run specific backend test suite
ginkgo -focus="Health Check Repository" ./tests/integration

# Run E2E acceptance tests (from repository root)
cd ../tests
export TEST_DATABASE_URL="postgres://postgres:postgres@localhost:5432/teams360_test?sslmode=disable"
ginkgo -v -focus="E2E: Survey Submission Flow" acceptance/

# Start backend server
go run cmd/api/main.go

# Build for production
go build -o bin/team360-api cmd/api/main.go

# Run with hot reload (using air)
air
```

## Development Commands

### Running the Full Stack

**Frontend** (Next.js):
```bash
npm install           # Install dependencies
npm run dev          # Start development server (http://localhost:3000)
npm run build        # Production build
npm start            # Start production server
```

**Backend** (Go/Gin):
```bash
cd backend
go mod download      # Install dependencies
go run cmd/api/main.go  # Start API server (http://localhost:8080)
ginkgo -v ./...      # Run all tests
```

**Root Makefile** (orchestrates both):
```bash
make dev             # Run both frontend and backend in parallel
make install         # Install all dependencies
make build           # Build both services
make test            # Run all tests (frontend + backend)
make clean           # Clean all artifacts
```

### Mac ARM64 Issues
If encountering SWC-related errors on Mac ARM64:
```bash
npm cache clean --force
rm -rf node_modules package-lock.json .next
npm install
npm install --force @next/swc-darwin-arm64  # If still having issues
```

## KubeVela + CNPG Deployment (Kubernetes)

Team360 supports deployment to Kubernetes via **KubeVela** (Open Application Model) with **CloudNativePG** for production-grade PostgreSQL. This is an alternative to the existing KubeVela deployment configs in `kubevela/`.

### Prerequisites

- **Docker** (for building images)
- **k3d** (lightweight k3s-in-Docker for local clusters)
- **kubectl** (Kubernetes CLI)
- **Helm** (installed automatically for operator setup)
- **KubeVela CLI** (`vela`) — installed automatically by Makefile targets
- `/etc/hosts` entry: `127.0.0.1 teams360.local`

### Quick Start

```bash
# Full pipeline: create cluster, install operators, build images, deploy
make kubevela-deploy-all

# Monitor deployment (CNPG needs ~45s to bootstrap)
make kubevela-status

# Access the app
open http://teams360.local:8080/      # Frontend
curl http://teams360.local:8080/api/v1/health-dimensions  # API

# Quick rebuild after code changes (skip cluster/operator setup)
make kubevela-deploy-quick

# Tear down
make kubevela-delete        # Remove application
make kubevela-k3d-delete    # Remove cluster
```

### Architecture Overview

| File | Purpose |
|------|---------|
| `kubevela/components/cnpg.cue` | CUE definition: `cloud-native-postgres` component type (renders CNPG Cluster CR) |
| `kubevela/components/gateway.cue` | CUE definition: `gateway` trait (renders Service + Ingress, k3d/Traefik only — production deployments should define their own ingress) |
| `kubevela/teams360-kubevela.yaml` | KubeVela Application manifest (3 components + ordered workflow) |
| `Makefile.kubevela` | All `kubevela-*` Make targets for automation |

**Deployment workflow** (defined in the YAML):
1. `deploy-database` — CNPG PostgreSQL cluster (1 instance, 0.5Gi)
2. `wait-for-infrastructure` — 45s suspend for CNPG to create cluster + secrets
3. `deploy-backend` — Go API (gets `DATABASE_URL` from CNPG secret via service-binding)
4. `deploy-frontend` — Next.js app (gets `BACKEND_URL=http://backend:8080`)

### Available Makefile Targets

| Target | Purpose |
|--------|---------|
| `kubevela-deploy-all` | Full pipeline: cluster + operators + build + deploy |
| `kubevela-deploy-quick` | Rebuild images and redeploy (assumes cluster exists) |
| `kubevela-k3d-create` | Create k3d cluster with port 8080:80 |
| `kubevela-k3d-delete` | Delete the k3d cluster |
| `kubevela-k3d-status` | Show cluster info and nodes |
| `kubevela-check-hosts` | Verify `/etc/hosts` has `teams360.local` |
| `kubevela-check-install-kubevela` | Install KubeVela CLI + operator |
| `kubevela-check-install-cnpg` | Install CNPG operator |
| `kubevela-build-and-load-images` | Docker build both images + k3d import |
| `kubevela-deploy` | Apply CUE defs + kubectl apply YAML |
| `kubevela-delete` | Remove the KubeVela application |
| `kubevela-status` | Show pods, services, ingress, CNPG clusters |

### Verification Checklist

After `make kubevela-deploy-all`, verify:

1. `make kubevela-deploy-all` completes from clean state
2. CNPG operator running: `kubectl get pods -n cnpg-system`
3. KubeVela operator running: `kubectl get pods -n vela-system`
4. PostgreSQL pod(s) running: `kubectl get pods -n teams360 -l cnpg.io/cluster=postgres`
5. Backend pod running and healthy: `kubectl get pods -n teams360 -l app.oam.dev/component=backend`
6. Frontend pod running and healthy: `kubectl get pods -n teams360 -l app.oam.dev/component=frontend`
7. Frontend loads: `curl -s http://teams360.local:8080/`
8. API returns data: `curl -s http://teams360.local:8080/api/v1/health-dimensions`
9. Login with `demo/demo` works through the browser
10. `make kubevela-delete` cleanly removes the app
11. `make kubevela-k3d-delete` removes the cluster

## Testing Strategy & End-to-End Testing

### Testing Philosophy: TRUE Outer-Loop TDD

Team360 follows **TRUE Outer-Loop Test-Driven Development** principles:

1. **Write E2E tests FIRST** that interact through the UI like a real user would
2. **Let tests drive implementation** - tests should fail until feature is complete
3. **Test the full application stack** - no mocking of servers, routes, or HTTP layers

**Why This Matters**: A bug where routes weren't registered in `main.go` taught us that integration tests using `httptest` bypass critical configuration. Only TRUE E2E tests that start the real server can catch these issues.

### Test Organization

Team360 uses a three-tier testing pyramid:

```
/tests/acceptance/              # E2E/Acceptance Tests (Playwright, full stack)
├── suite_test.go              # Test suite setup - starts servers & Playwright
├── e2e_authentication_test.go # Login flow E2E test
├── e2e_survey_submission_test.go # Survey submission E2E test
└── e2e_manager_dashboard_test.go # Manager dashboard E2E test

/backend/tests/integration/     # Integration Tests (API handlers with httptest)
├── auth_test.go               # Authentication API integration tests
└── manager_dashboard_test.go  # Manager API integration tests

/backend/{domain}/              # Unit Tests (in module directories)
├── interfaces/api/v1/health_check_api_test.go    # API handler unit tests
└── infrastructure/persistence/postgres/
    ├── health_check_repository_test.go           # Repository unit tests
    └── database_migration_test.go                # Migration unit tests
```

**Key Principle**: Tests in `/tests/acceptance/` MUST be TRUE E2E tests that use Playwright. Tests that use `httptest` belong in module directories.

### TRUE E2E Testing Principles

**What Makes a Test "TRUE E2E"?**

✅ **CORRECT E2E Test**:
- Starts FULL application stack (backend Go server + frontend Next.js server + database)
- Uses Playwright to drive a REAL browser (Chromium/Firefox/WebKit)
- Interacts through UI elements like a real user (clicks, fills forms, navigates)
- Verifies complete workflows end-to-end
- Checks database to confirm data persistence

❌ **NOT E2E** (These are integration/unit tests):
- Uses `httptest.NewRequest()` to call handlers directly
- Creates a test Gin router and registers routes programmatically
- Bypasses actual server startup and route registration in `main.go`
- Mocks HTTP requests or server responses

### Example: TRUE E2E Test

From `/tests/acceptance/e2e_manager_dashboard_test.go`:

```go
var _ = Describe("E2E: Manager Dashboard", func() {
    var page playwright.Page

    BeforeEach(func() {
        var err error
        page, err = browser.NewPage()
        Expect(err).NotTo(HaveOccurred())
    })

    AfterEach(func() {
        page.Close()
    })

    Context("when manager views their team's health", func() {
        It("should return aggregated health data for assigned teams", func() {
            // Given: Test data exists in database
            By("Inserting test manager and team data")
            _, err := db.Exec(`
                INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash)
                VALUES ('manager1', 'manager1', 'manager1@test.com', 'Test Manager', 'level-3', 'demo')
            `)
            Expect(err).NotTo(HaveOccurred())

            // When: Manager logs in via UI
            By("Manager logging in to the application")
            _, err = page.Goto(frontendURL + "/login")
            Expect(err).NotTo(HaveOccurred())

            err = page.Locator("input[name='username']").Fill("manager1")
            Expect(err).NotTo(HaveOccurred())

            err = page.Locator("input[name='password']").Fill("demo")
            Expect(err).NotTo(HaveOccurred())

            err = page.Locator("button[type='submit']").Click()
            Expect(err).NotTo(HaveOccurred())

            // Then: Should be redirected to manager dashboard
            By("Verifying redirect to manager dashboard")
            Eventually(func() string {
                return page.URL()
            }, 10*time.Second, 500*time.Millisecond).Should(ContainSubstring("/manager"))

            // And: Dashboard should show team health data
            By("Verifying dashboard displays team health")
            Eventually(func() bool {
                teamCard := page.Locator("[data-testid='team-card']").First()
                visible, _ := teamCard.IsVisible()
                return visible
            }, 10*time.Second, 500*time.Millisecond).Should(BeTrue())
        })
    })
})
```

**Key Observations**:
1. Starts with real login page at `frontendURL + "/login"`
2. Fills form inputs like a user: `page.Locator("input[name='username']").Fill(...)`
3. Clicks submit button: `page.Locator("button[type='submit']").Click()`
4. Waits for navigation: `Eventually(func() string { return page.URL() }...)`
5. Verifies UI elements render: `page.Locator("[data-testid='team-card']").IsVisible()`

### Test Suite Setup: Starting the Full Stack

The `/tests/acceptance/suite_test.go` file orchestrates the E2E test environment:

**What It Does**:
1. **Creates test database** and runs migrations
2. **Starts backend Go server** on localhost:8080
3. **Starts frontend Next.js server** on localhost:3000
4. **Initializes Playwright** and launches browser
5. **Waits for both servers** to be ready before running tests
6. **Cleans up** - stops servers and closes browser after all tests

**Key Code** (simplified):

```go
var _ = SynchronizedBeforeSuite(
    // Phase 1: Runs ONCE on process #1 - setup database and servers
    func() []byte {
        By("Setting up test database and running migrations")
        // ... database setup code ...

        By("Starting backend API server")
        backendCmd = exec.Command("go", "run", "cmd/api/main.go")
        backendCmd.Start()

        // Wait for backend to be ready
        Eventually(func() error {
            resp, _ := exec.Command("curl", backendURL+"/health").Output()
            if string(resp) != "200" { return fmt.Errorf("not ready") }
            return nil
        }, 30*time.Second).Should(Succeed())

        By("Starting frontend Next.js server")
        frontendCmd = exec.Command("npm", "run", "dev")
        frontendCmd.Start()

        // Wait for frontend to be ready
        Eventually(func() error {
            resp, _ := exec.Command("curl", frontendURL).Output()
            if string(resp) != "200" { return fmt.Errorf("not ready") }
            return nil
        }, 60*time.Second).Should(Succeed())

        return []byte(databaseURL)
    },

    // Phase 2: Runs on ALL processes - setup Playwright
    func(databaseURL []byte) {
        By("Initializing Playwright browser for this process")
        pw, _ = playwright.Run()
        browser, _ = pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
            Headless: playwright.Bool(true),
        })
    },
)
```

**This is CRITICAL**: The test suite starts the REAL servers, not test doubles. This is what makes these TRUE E2E tests.

### Running E2E Tests

**Run all E2E tests**:
```bash
cd $(git rev-parse --show-toplevel)
export TEST_DATABASE_URL="postgres://postgres:postgres@localhost:5432/teams360_test?sslmode=disable"
ginkgo -v tests/acceptance/
```

**Run specific E2E test suite**:
```bash
# Run only authentication tests
ginkgo -v -focus="E2E: Authentication" tests/acceptance/

# Run only manager dashboard tests
ginkgo -v -focus="E2E: Manager Dashboard" tests/acceptance/

# Run only survey submission tests
ginkgo -v -focus="E2E: Survey Submission" tests/acceptance/
```

**What to expect**:
- Tests take 30-60 seconds to start (servers must boot)
- You'll see backend and frontend logs in test output
- Playwright runs in headless mode (no browser window)
- Tests will fail if servers don't start or database isn't accessible

### Why TRUE E2E Tests Caught the Bug

**The Bug**: Manager dashboard routes existed in code but weren't registered in `main.go`:

```go
// backend/cmd/api/main.go was MISSING this line:
v1.SetupManagerRoutes(router, db)  // Without this, routes return 404
```

**Why Integration Tests Missed It**:
Integration tests using `httptest` created their own test router and registered routes programmatically:

```go
// Integration test (DOESN'T test main.go):
router := gin.New()
v1.SetupManagerRoutes(router, db)  // Test registers routes itself!
req := httptest.NewRequest("GET", "/api/v1/managers/mgr1/teams/health", nil)
router.ServeHTTP(w, req)  // Bypasses main.go entirely
```

**Why E2E Tests Caught It**:
E2E tests start the real server by running `go run cmd/api/main.go`:

```go
// E2E test (TESTS real main.go):
backendCmd = exec.Command("go", "run", "cmd/api/main.go")  // Runs actual main.go!
backendCmd.Start()

// User logs in via browser and navigates to /manager
page.Goto(frontendURL + "/login")
page.Fill("input[name='username']", "manager1")
page.Click("button[type='submit']")

// Frontend makes real HTTP request to backend
fetch('/api/v1/managers/manager1/teams/health')  // Returns 404 if routes not registered!
```

**Lesson**: Only tests that start the real server can catch configuration issues in `main.go`.

### Adding E2E Tests for New Features

When implementing new features with Outer-Loop TDD:

1. **Start with E2E test** in `/tests/acceptance/`
2. **Describe user workflow** (Given-When-Then)
3. **Use Playwright** to interact through browser
4. **Run test** - it should FAIL (Red)
5. **Implement feature** until test passes (Green)
6. **Refactor** with confidence

**Template**:

```go
var _ = Describe("E2E: [Feature Name]", func() {
    var page playwright.Page

    BeforeEach(func() {
        var err error
        page, err = browser.NewPage()
        Expect(err).NotTo(HaveOccurred())
    })

    AfterEach(func() {
        page.Close()
    })

    Context("when user [performs action]", func() {
        It("should [expected outcome]", func() {
            // Given: Setup test data
            By("Setting up test data")
            // ... insert test data into database ...

            // When: User performs action through UI
            By("User [action description]")
            _, err := page.Goto(frontendURL + "/path")
            Expect(err).NotTo(HaveOccurred())
            // ... interact with UI using Playwright ...

            // Then: Verify expected outcome
            By("Verifying [expected result]")
            // ... assert UI state and/or database state ...
        })
    })
})
```

### Best Practices

1. **Use semantic selectors**: `page.Locator("[data-testid='team-card']")` is better than CSS classes
2. **Add data-testid attributes**: Makes tests resilient to UI changes
3. **Use Eventually()**: Wait for async operations: `Eventually(func() bool { ... }).Should(BeTrue())`
4. **Clean test data**: Use `BeforeEach` to ensure clean state
5. **Test full workflows**: Don't just test happy path - test error cases via UI too
6. **Verify database**: Check that data persists correctly after UI actions

## Authentication & Demo Credentials

Authentication is cookie-based using js-cookie. All demo passwords are "demo" except admin ("admin").

Test users by hierarchy level:
- VP: `vp/demo`
- Directors: `director1/demo`, `director2/demo`
- Managers: `manager1/demo`, `manager2/demo`, `manager3/demo`
- Team Leads: `teamlead1/demo` through `teamlead9/demo`
- Team Members: `demo/demo`, `alice/demo`, etc.
- Admin: `admin/admin`

## Architecture & Key Concepts

### Data Architecture

The application uses **localStorage for persistence** in lieu of a database:
- **Health check sessions**: Stored in `localStorage` key `healthCheckSessions`
- **Organization config**: Stored in `localStorage` key `orgConfig`
- **User authentication**: Stored in cookies via js-cookie (expires in 1 day)

This is a demo application - all data is client-side and resets on localStorage clear.

### Core Data Models

Located in `frontend/lib/types.ts`:

1. **Hierarchy System**: Configurable organizational levels with granular permissions
   - `HierarchyLevel`: Defines levels (VP, Director, Manager, Team Lead, Team Member)
   - `OrganizationConfig`: Company-wide hierarchy configuration
   - Each level has specific permissions (canViewAllTeams, canEditTeams, etc.)

2. **Teams & Users**:
   - `User`: Has hierarchyLevelId, reportsTo (supervisor), and teamIds (can be in multiple teams)
   - `Team`: Has supervisorChain (full chain of supervisors), members, cadence (survey frequency)

3. **Health Checks**:
   - `HealthDimension`: 11 dimensions based on Spotify's model with Team360 enhancements:
     - **From Spotify's model**: Mission, Delivering Value, Speed, Fun, Health of Codebase, Learning, Support, Pawns or Players
     - **Team360 additions**: Easy to Release, Suitable Process, Teamwork
     - Each dimension has goodDescription/badDescription for clarity (e.g., "We deliver great stuff!" vs "We deliver crap")
     - Dimensions can be enabled/disabled and weighted via isActive and weight properties
   - `HealthCheckSession`: User's responses to health check, includes assessmentPeriod (e.g., "2024 - 1st Half")
   - `HealthCheckResponse`: Score (1=red, 2=yellow, 3=green), trend (improving/stable/declining), optional comment

### Assessment Period Logic

**Automatic Detection** (implemented in `frontend/lib/assessment-period.ts`):
- Jan 1 - Jun 30 → "previous year - 2nd Half" (e.g., 2025-01-15 → "2024 - 2nd Half")
- Jul 1 - Dec 31 → "current year - 1st Half" (e.g., 2025-07-15 → "2025 - 1st Half")
- Eliminates manual period selection in surveys
- Enables automatic trend tracking across periods

```typescript
export function getAssessmentPeriod(date?: Date | string): string {
  const submissionDate = date ? (typeof date === 'string' ? new Date(date) : date) : new Date();
  const month = submissionDate.getMonth(); // 0-indexed
  const year = submissionDate.getFullYear();

  if (month >= 0 && month <= 5) {
    return `${year - 1} - 2nd Half`;
  } else {
    return `${year} - 1st Half`;
  }
}
```

### Supervisor Chain & Access Control

Teams have a `supervisorChain` array that defines the full reporting hierarchy:
```typescript
supervisorChain: [
  { userId: 'lead1', levelId: 'level-4' },  // Team Lead
  { userId: 'mgr1', levelId: 'level-3' },   // Manager
  { userId: 'dir1', levelId: 'level-2' },   // Director
  { userId: 'vp1', levelId: 'level-1' }     // VP
]
```

Access control logic (in `frontend/lib/org-config.ts`):
- `getUserPermissions()`: Returns permissions based on user's hierarchy level
- `canUserAccessTeam()`: Checks if user can view a team (based on permissions, membership, or supervisor chain)
- `getSubordinates()`: Recursively gets all users reporting to a given user

### Key Data Files

- `frontend/lib/auth.ts`: Authentication logic and USERS array (45+ demo users)
- `frontend/lib/data.ts`: HEALTH_DIMENSIONS (11 dimensions), TEAMS array (9 squads), health check sessions
- `frontend/lib/teams-data.ts`: Extended TEAMS_DATA with supervisor chains, mock session generator
- `frontend/lib/org-config.ts`: Organization hierarchy configuration and permission system
- `frontend/lib/assessment-period.ts`: Automatic assessment period detection utility

### Route Structure

- `/` - Landing page (public)
- `/login` - Authentication (public)
- `/survey` - Health check survey (Team Members and up) - **11 questions, no manual period selection**
- `/dashboard` - Team Lead dashboard (Team Leads only)
- `/manager` - Manager/Director/VP dashboard with team filtering and analytics
- `/admin` - System administration (Admin only)

`frontend/middleware.ts` handles route protection and role-based redirects based on user cookie.

### State Management Pattern

The application uses **client-side state with localStorage persistence**:

1. Data is initialized from mock data arrays (USERS, TEAMS, HEALTH_DIMENSIONS)
2. On first load, data may be populated from localStorage if available
3. When data changes (e.g., completing a survey), it's updated in memory and saved to localStorage
4. Pattern in `frontend/lib/data.ts`:
   ```typescript
   let healthCheckSessions: HealthCheckSession[] = [];
   const stored = localStorage.getItem('healthCheckSessions');
   // ... load from storage

   export const saveHealthCheckSession = (session: HealthCheckSession) => {
     // ... update in memory
     localStorage.setItem('healthCheckSessions', JSON.stringify(healthCheckSessions));
   };
   ```

### Manager Dashboard Filtering

The manager dashboard (`frontend/app/manager/page.tsx`) implements hierarchical team filtering:
- Users see only teams they have access to (based on permissions and supervisor chain)
- Managers see teams where they appear in the supervisorChain
- Directors and VPs see teams of all their subordinates
- Filtering logic uses `canUserAccessTeam()` from `frontend/lib/org-config.ts`

### Assessment Periods & Trend Lines

Health check sessions can be tagged with an `assessmentPeriod` (e.g., "2024 - 1st Half"). The Team Lead dashboard uses this to:
- Filter trend data by assessment period
- Show period-specific trend lines on charts
- Allow comparison across different time periods

Implementation in `frontend/app/dashboard/page.tsx` uses Recharts LineChart with period-based data filtering.

## Important Implementation Details

### TypeScript Path Aliases
Uses `@/*` for imports (configured in `tsconfig.json`):
```typescript
import { User } from '@/lib/types';
import { getAssessmentPeriod } from '@/lib/assessment-period';
```

### Health Check Score Mapping
- Score 1 (Red) = Poor health
- Score 2 (Yellow) = Medium health
- Score 3 (Green) = Good health

### Data Visualization
Uses Recharts library extensively:
- RadarChart for overall team health
- BarChart for response distributions
- LineChart for trends over time
- Color scheme: Red (#EF4444), Yellow (#F59E0B), Green (#10B981)

### Team Assignments Version
`frontend/lib/data.ts` exports `TEAM_ASSIGNMENTS_VERSION` constant - increment this when changing manager-team assignments to trigger re-initialization of cached data.

## Common Development Patterns

When adding features:

1. **New health dimensions**: Update `HEALTH_DIMENSIONS` in `frontend/lib/data.ts` (currently 11 dimensions)
2. **New hierarchy levels**: Use functions in `frontend/lib/org-config.ts` (`addHierarchyLevel`, `updateHierarchyLevel`)
3. **Access control**: Always check permissions with `getUserPermissions()` and `canUserAccessTeam()`
4. **Data persistence**: Remember to update localStorage when modifying sessions or config
5. **Mock data generation**: See `generateMockHealthSessions()` in `frontend/lib/teams-data.ts` for patterns
6. **Assessment periods**: Use `getAssessmentPeriod()` from `frontend/lib/assessment-period.ts` for automatic detection

## Technology Stack

**Frontend**:
- **Framework**: Next.js 15 with App Router
- **Language**: TypeScript (strict mode)
- **Styling**: Tailwind CSS
- **Charts**: Recharts
- **Icons**: Lucide React
- **Forms**: React Hook Form
- **State**: Client-side with localStorage (will migrate to API calls)
- **Auth**: Cookie-based with js-cookie (will migrate to JWT/session tokens)

**Backend** (In Development):
- **Language**: Go 1.25 (use latest stable as of October 2025)
- **Framework**: Gin (latest version - HTTP router and middleware)
- **Architecture**: Domain-Driven Design (DDD)
- **Testing**: Ginkgo v2 (latest - BDD framework) + Gomega (latest - assertions)
- **Database**: TBD (PostgreSQL recommended for production)
- **ORM**: TBD (GORM or sqlx recommended - use latest versions)
- **Auth**: JWT tokens or session-based (to be implemented)
- **API**: RESTful JSON API (versioned at /api/v1/)

**Important**: Always use the latest stable versions of all Go dependencies as of October 2025. Check `backend/go.mod` for specific pinned versions.

## Open Source Philosophy

Team360 is released under the **Apache 2.0 license** to benefit organizations worldwide. The goal is to make structured team health assessment accessible to any organization, regardless of size or resources.

### Design Principles

When contributing or extending Team360, keep these principles in mind:

1. **Support, Not Surveillance**: Features should help teams improve, never be weaponized for performance reviews
2. **Flexibility First**: Organizations differ - support customization (dimensions, cadences, hierarchies)
3. **Trust-Based Design**: The system assumes honest input; don't build features that incentivize gaming metrics
4. **Privacy Conscious**: Individual responses should inform team aggregates, but protect contributor anonymity where appropriate
5. **Accessibility**: Keep the demo simple - localStorage and mock data allow anyone to try it without backend setup

### Current State & Roadmap

**Current (Demo Phase)**:
- Frontend: Fully functional Next.js app with mock data and localStorage
- Backend: **In Development** - Go 1.25/Gin backend with DDD architecture

**Backend Development Guidelines**:
- Use Go 1.25 as the minimum version
- Apply outer-loop TDD: Write Ginkgo acceptance tests first, then implement
- Follow DDD structure: domain → application → infrastructure → interfaces
- Use latest stable versions of all dependencies (Gin, Ginkgo, Gomega, etc.)
- Maintain clean architecture boundaries

**Migration Path** (Frontend → Backend):
1. **Phase 1**: Set up Go backend structure with DDD layers (domain/application/infrastructure/interfaces) ✅ COMPLETE
2. **Phase 2**: Implement domain models (User, Team, HealthCheck aggregates) with Ginkgo tests
3. **Phase 3**: Build API endpoints following TDD with Ginkgo/Gomega
4. **Phase 4**: Add database persistence layer (PostgreSQL + GORM/sqlx)
5. **Phase 5**: Migrate frontend from localStorage to API calls
6. **Phase 6**: Implement proper authentication (JWT or session-based)
7. **Phase 7**: Add export features (CSV/Excel)
8. **Phase 8**: Implement notifications (email/Slack)

**Key Backend Features to Build**:
- RESTful API at `/api/v1/` with Gin
- Repository pattern for data access
- Domain events for cross-aggregate communication
- Comprehensive test coverage with Ginkgo/Gomega

## Code Style & Best Practices

- **Modular Design**: Files under 500 lines
- **Environment Safety**: Never hardcode secrets
- **Test-First**: Write tests before implementation (TDD with Ginkgo/Gomega)
- **Clean Architecture**: Separate concerns (DDD layers)
- **Documentation**: Keep CLAUDE.md updated with architectural decisions

# important-instruction-reminders
Do what has been asked; nothing more, nothing less.
NEVER create files unless they're absolutely necessary for achieving your goal.
ALWAYS prefer editing an existing file to creating a new one.
NEVER proactively create documentation files (*.md) or README files. Only create documentation files if explicitly requested by the User.
Never save working files, text/mds and tests to the root folder (except CLAUDE.md which is project documentation).
