# Testing Guide

This document outlines the comprehensive testing strategy for the Authway project, including frontend and backend test coverage to achieve 100% code coverage.

## Overview

The project includes comprehensive test coverage for both frontend React components and backend Go services, with the following testing infrastructure:

### Frontend Testing Stack
- **Vitest**: Modern test runner and assertion library
- **React Testing Library**: Component testing utilities
- **MSW v2**: API mocking for integration tests
- **JSDOM**: Browser environment simulation
- **User Event**: Real user interaction simulation

### Backend Testing Stack
- **Go Testing**: Native Go testing framework
- **Testify**: Assertion and mocking library
- **SQLite**: In-memory database for service layer tests
- **Zaptest**: Logger testing utilities

## Test Structure

```
├── packages/web/login-ui/
│   ├── src/test/
│   │   ├── setup.ts          # Test environment setup
│   │   ├── utils.tsx         # Test utilities and providers
│   │   └── mocks/
│   │       └── server.ts     # MSW server setup
│   ├── src/pages/
│   │   ├── LoginPage.test.tsx
│   │   ├── ConsentPage.test.tsx
│   │   └── RegisterPage.test.tsx
│   └── vitest.config.ts      # Vitest configuration
│
├── packages/web/admin-dashboard/
│   ├── vitest.config.ts      # Vitest configuration
│   └── src/               # Test files alongside components
│
└── src/server/
    ├── pkg/user/
    │   └── service_test.go   # User service tests
    ├── pkg/auth/
    │   └── service_test.go   # Auth service tests
    ├── pkg/client/
    │   └── service_test.go   # Client service tests
    └── internal/handler/
        └── auth_test.go      # Auth handler tests
```

## Coverage Goals

- **Target**: 100% test coverage across all components and services
- **Frontend**: All React components, hooks, and utilities
- **Backend**: All services, handlers, and business logic

### Coverage Thresholds (Vitest)

```javascript
coverage: {
  thresholds: {
    global: {
      branches: 100,
      functions: 100,
      lines: 100,
      statements: 100
    }
  }
}
```

## Running Tests

### Frontend Tests

```bash
# Run login-ui tests
cd packages/web/login-ui
npm test

# Run tests with coverage
npm run test:coverage

# Run admin dashboard tests
cd packages/web/admin-dashboard
npm test
```

### Backend Tests

```bash
# Run all Go tests
go test ./src/server/...

# Run tests with coverage
go test -cover ./src/server/...

# Run specific package tests
go test ./src/server/pkg/user
go test ./src/server/pkg/auth
go test ./src/server/pkg/client
go test ./src/server/internal/handler
```

## Test Categories

### 1. Unit Tests
- **Frontend**: Individual component behavior, hooks, utilities
- **Backend**: Service layer methods, business logic functions

### 2. Integration Tests
- **Frontend**: Component interactions with mocked APIs
- **Backend**: Handler-service integration with real database operations

### 3. API Mock Tests
- **MSW Handlers**: All API endpoints mocked for frontend testing
- **Authentication flows**: Login, consent, registration
- **Error scenarios**: Network failures, validation errors

## Frontend Test Patterns

### Component Testing
```typescript
// Example from LoginPage.test.tsx
describe('LoginPage', () => {
  it('shows loading spinner initially', () => {
    render(<LoginPage />)
    expect(screen.getByTestId('loading-spinner')).toBeInTheDocument()
  })

  it('handles successful login with redirect', async () => {
    // Mock API response
    server.use(
      http.post('http://localhost:8080/login', () => {
        return HttpResponse.json({ redirect_to: 'http://example.com/callback' })
      })
    )

    // Test user interaction and assertions
    const user = userEvent.setup()
    // ... test implementation
  })
})
```

### MSW API Mocking
```typescript
// Example from server.ts
export const handlers = [
  http.post('http://localhost:8080/login', () => {
    return HttpResponse.json({
      redirect_to: 'http://localhost:3000/callback'
    })
  }),

  http.get('http://localhost:8080/consent', ({ request }) => {
    const url = new URL(request.url)
    const challenge = url.searchParams.get('consent_challenge')
    return HttpResponse.json({
      challenge,
      client_name: 'Test Application',
      requested_scope: ['openid', 'email'],
      user: mockUser
    })
  })
]
```

## Backend Test Patterns

### Service Layer Testing
```go
// Example from service_test.go
func TestService_Create(t *testing.T) {
    db := setupTestDB(t)
    logger := zaptest.NewLogger(t)
    service := NewService(db, logger)

    tests := []struct {
        name        string
        request     *CreateUserRequest
        expectError bool
        errorMsg    string
    }{
        {
            name: "successful user creation",
            request: &CreateUserRequest{
                Email:     "test@example.com",
                Password:  "password123",
                FirstName: "John",
                LastName:  "Doe",
            },
            expectError: false,
        },
        // ... more test cases
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            user, err := service.Create(tt.request)
            // ... assertions
        })
    }
}
```

### Handler Testing with Mocks
```go
// Example from auth_test.go using testify mocks
type MockUserService struct {
    mock.Mock
}

func (m *MockUserService) GetByEmail(email string) (*user.User, error) {
    args := m.Called(email)
    if args.Get(0) == nil {
        return nil, args.Error(1)
    }
    return args.Get(0).(*user.User), args.Error(1)
}

func TestAuthHandler_Login(t *testing.T) {
    mockUserService := &MockUserService{}
    handler := NewAuthHandler(mockUserService, mockHydraClient)

    // Setup mocks
    mockUserService.On("GetByEmail", "test@example.com").Return(testUser, nil)

    // Test HTTP handler
    // ... test implementation
}
```

## Test Data Management

### Frontend Mock Data
- User profiles with various states (verified, unverified, active, inactive)
- OAuth clients with different configurations
- API responses for success and error scenarios

### Backend Test Database
- In-memory SQLite for fast, isolated tests
- Automatic schema migration for each test
- Test data cleanup between tests

## Known Issues and Workarounds

### 1. Go SQLite CGO Dependency
**Issue**: SQLite driver requires CGO, which may not be available in all environments.

**Current Status**: Tests written but may fail without CGO_ENABLED=1 and C compiler.

**Workarounds**:
- Use Docker for consistent testing environment
- Mock database layer for unit tests
- Use GitHub Actions with CGO enabled for CI/CD

### 2. MSW v2 Migration
**Status**: Successfully migrated from MSW v1 to v2 syntax.
- Changed `rest.post()` to `http.post()`
- Updated response format to use `HttpResponse.json()`

### 3. Missing Component Data Attributes
**Fixed**: Added `data-testid` attributes to components for reliable test selection.

## CI/CD Integration

### GitHub Actions Workflow (Recommended)

```yaml
name: Tests
on: [push, pull_request]

jobs:
  frontend-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-node@v3
        with:
          node-version: '18'
      - run: npm ci
      - run: npm run test:coverage
      - uses: codecov/codecov-action@v3

  backend-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: '1.21'
      - run: go test -race -coverprofile=coverage.out ./...
      - uses: codecov/codecov-action@v3
```

## Coverage Reports

### Frontend Coverage
```bash
cd packages/web/login-ui
npm run test:coverage
# Generates coverage/index.html
```

### Backend Coverage
```bash
go test -coverprofile=coverage.out ./src/server/...
go tool cover -html=coverage.out -o coverage.html
```

## Best Practices

### Frontend
1. **Test user behavior, not implementation details**
2. **Use realistic test data and scenarios**
3. **Mock external dependencies consistently**
4. **Test both happy path and error scenarios**

### Backend
1. **Use table-driven tests for comprehensive coverage**
2. **Test with real database operations where possible**
3. **Mock external services (Hydra, Redis) appropriately**
4. **Verify side effects (logging, database changes)**

## Maintenance

### Adding New Tests
1. Follow existing patterns and structure
2. Update MSW handlers for new API endpoints
3. Ensure 100% coverage for new components/services
4. Add edge case and error scenario tests

### Updating Dependencies
1. Monitor for breaking changes in testing libraries
2. Update mock data to match API changes
3. Verify coverage thresholds after updates

## Summary

The Authway project has comprehensive test coverage including:

✅ **Frontend**: React component tests with MSW API mocking
✅ **Backend**: Service layer tests with in-memory database
✅ **Integration**: Handler tests with mocked dependencies
✅ **Coverage**: 100% coverage target with threshold enforcement
🔧 **CI/CD Ready**: Configuration for automated testing workflows

**Current Status**: Testing infrastructure is complete and ready for 100% coverage achievement once database dependency issues are resolved.