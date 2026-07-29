---
name: write-e2e-test
description: Write TRUE E2E acceptance tests using Playwright and Ginkgo for Team360. Use this when adding new E2E tests or implementing features with outer-loop TDD.
---

# Write E2E Test for Team360

## Testing Philosophy: TRUE Outer-Loop TDD

Team360 follows **TRUE Outer-Loop Test-Driven Development**:

1. **Write E2E tests FIRST** that interact through the UI like a real user would
2. **Let tests drive implementation** - tests should fail until feature is complete
3. **Test the full application stack** - no mocking of servers, routes, or HTTP layers

**Why This Matters**: A bug where routes weren't registered in `main.go` taught us that integration tests using `httptest` bypass critical configuration. Only TRUE E2E tests that start the real server can catch these issues.

## Test Organization

```
/tests/acceptance/              # E2E/Acceptance Tests (Playwright, full stack)
├── suite_test.go              # Test suite setup - starts servers & Playwright
├── e2e_authentication_test.go # Login flow E2E test
├── e2e_survey_submission_test.go # Survey submission E2E test
└── e2e_manager_dashboard_test.go # Manager dashboard E2E test

/backend/tests/integration/     # Integration Tests (API handlers with httptest)
/backend/{domain}/              # Unit Tests (in module directories)
```

**Key Principle**: Tests in `/tests/acceptance/` MUST be TRUE E2E tests that use Playwright. Tests that use `httptest` belong in module directories.

## What Makes a Test "TRUE E2E"

**CORRECT E2E Test**:
- Starts FULL application stack (backend Go server + frontend Next.js server + database)
- Uses Playwright to drive a REAL browser (Chromium/Firefox/WebKit)
- Interacts through UI elements like a real user (clicks, fills forms, navigates)
- Verifies complete workflows end-to-end
- Checks database to confirm data persistence

**NOT E2E** (These are integration/unit tests):
- Uses `httptest.NewRequest()` to call handlers directly
- Creates a test Gin router and registers routes programmatically
- Bypasses actual server startup and route registration in `main.go`
- Mocks HTTP requests or server responses

## E2E Test Template

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

## Example: Manager Dashboard E2E Test

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
            By("Inserting test manager and team data")
            _, err := db.Exec(`
                INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash)
                VALUES ('manager1', 'manager1', 'manager1@test.com', 'Test Manager', 'level-3', 'demo')
            `)
            Expect(err).NotTo(HaveOccurred())

            By("Manager logging in to the application")
            _, err = page.Goto(frontendURL + "/login")
            Expect(err).NotTo(HaveOccurred())

            err = page.Locator("input[name='username']").Fill("manager1")
            Expect(err).NotTo(HaveOccurred())

            err = page.Locator("input[name='password']").Fill("demo")
            Expect(err).NotTo(HaveOccurred())

            err = page.Locator("button[type='submit']").Click()
            Expect(err).NotTo(HaveOccurred())

            By("Verifying redirect to manager dashboard")
            Eventually(func() string {
                return page.URL()
            }, 10*time.Second, 500*time.Millisecond).Should(ContainSubstring("/manager"))

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

## Test Suite Setup

The `/tests/acceptance/suite_test.go` orchestrates the E2E environment:

1. **Creates test database** and runs migrations
2. **Starts backend Go server** on localhost:8080
3. **Starts frontend Next.js server** on localhost:3000
4. **Initializes Playwright** and launches browser
5. **Waits for both servers** to be ready before running tests
6. **Cleans up** - stops servers and closes browser after all tests

The test suite starts the REAL servers, not test doubles. This is what makes these TRUE E2E tests.

## Running E2E Tests

```bash
cd /path/to/teams360
export TEST_DATABASE_URL="postgres://postgres:postgres@localhost:5432/teams360_test?sslmode=disable"

# Run all E2E tests
ginkgo -v tests/acceptance/

# Run specific test suite
ginkgo -v -focus="E2E: Authentication" tests/acceptance/
ginkgo -v -focus="E2E: Manager Dashboard" tests/acceptance/
ginkgo -v -focus="E2E: Survey Submission" tests/acceptance/
```

## Ginkgo Test Structure (Backend)

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

## Best Practices

1. **Use semantic selectors**: `page.Locator("[data-testid='team-card']")` over CSS classes
2. **Add data-testid attributes**: Makes tests resilient to UI changes
3. **Use Eventually()**: Wait for async operations: `Eventually(func() bool { ... }).Should(BeTrue())`
4. **Clean test data**: Use `BeforeEach` to ensure clean state
5. **Test full workflows**: Don't just test happy path - test error cases via UI too
6. **Verify database**: Check that data persists correctly after UI actions

## TDD Workflow

1. **Start with E2E test** in `/tests/acceptance/`
2. **Describe user workflow** (Given-When-Then)
3. **Use Playwright** to interact through browser
4. **Run test** - it should FAIL (Red)
5. **Implement feature** until test passes (Green)
6. **Refactor** with confidence

$ARGUMENTS
