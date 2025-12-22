# Contributing

This guide covers development setup, coding standards, and contribution workflow for the Chainlink Framework.

## Development Setup

### Prerequisites

- **Go 1.21+** with module support
- **Make** for build automation
- **Git** for version control

### Clone and Setup

```bash
git clone https://github.com/smartcontractkit/chainlink-framework.git
cd chainlink-framework

# Tidy all modules
make gomodtidy

# Generate mocks and module graph
make generate
```

### Project Structure

```
chainlink-framework/
├── capabilities/          # Chainlink capabilities (WriteTarget)
│   └── writetarget/       # Write target implementation
├── chains/                # Chain abstractions
│   ├── heads/             # Head tracking
│   ├── txmgr/             # Transaction manager
│   └── fees/              # Fee estimation
├── metrics/               # Prometheus metrics
├── multinode/             # Multi-RPC client
├── tools/                 # Build tools
├── Makefile               # Build automation
└── go.md                  # Module dependency graph
```

### Running Tests

```bash
# Run all tests
go test ./...

# Run tests for a specific module
cd multinode && go test ./...

# Run tests with coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

## Coding Standards

### Go Style

Follow the [Uber Go Style Guide](https://github.com/uber-go/guide/blob/master/style.md) with these specifics:

1. **Use meaningful names**: Avoid single-letter variables except in short loops
2. **Document exported types**: All exported types, functions, and methods need doc comments
3. **Handle errors explicitly**: Don't ignore errors; wrap with context
4. **Use structured logging**: Use `logger.SugaredLogger` with key-value pairs

```go
// Good
func (n *node) Start(ctx context.Context) error {
    n.lggr.Infow("Starting node", "name", n.name, "chainID", n.chainID)
    if err := n.dial(ctx); err != nil {
        return fmt.Errorf("failed to dial node %s: %w", n.name, err)
    }
    return nil
}

// Bad
func (n *node) Start(ctx context.Context) error {
    fmt.Println("starting")
    n.dial(ctx) // Error ignored
    return nil
}
```

### Generics Conventions

Use descriptive type parameter names:

```go
// Good - clear type parameter names
type MultiNode[
    CHAIN_ID ID,
    RPC any,
] struct { ... }

// Bad - unclear type parameters
type MultiNode[C, R any] struct { ... }
```

### Interface Design

Keep interfaces small and focused:

```go
// Good - single responsibility
type Dialer interface {
    Dial(ctx context.Context) error
}

type ChainIDGetter interface {
    ChainID(ctx context.Context) (ID, error)
}

// Compose when needed
type RPCClient interface {
    Dialer
    ChainIDGetter
    // ...
}
```

### Service Pattern

All long-running components should implement `services.Service`:

```go
type MyService struct {
    services.Service
    eng *services.Engine
}

func NewMyService(lggr logger.Logger) *MyService {
    s := &MyService{}
    s.Service, s.eng = services.Config{
        Name:  "MyService",
        Start: s.start,
        Close: s.close,
    }.NewServiceEngine(lggr)
    return s
}

func (s *MyService) start(ctx context.Context) error {
    s.eng.Go(s.runLoop)
    return nil
}

func (s *MyService) close() error {
    return nil
}
```

## Testing Guidelines

### Unit Tests

- Test public APIs primarily
- Use table-driven tests for multiple cases
- Mock external dependencies

```go
func TestNode_Start(t *testing.T) {
    tests := []struct {
        name    string
        setup   func(*mockRPC)
        wantErr bool
    }{
        {
            name: "successful start",
            setup: func(m *mockRPC) {
                m.On("Dial", mock.Anything).Return(nil)
                m.On("ChainID", mock.Anything).Return(big.NewInt(1), nil)
            },
            wantErr: false,
        },
        {
            name: "dial failure",
            setup: func(m *mockRPC) {
                m.On("Dial", mock.Anything).Return(errors.New("connection refused"))
            },
            wantErr: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            mockRPC := newMockRPC(t)
            tt.setup(mockRPC)

            node := NewNode(/* ... */, mockRPC, /* ... */)
            err := node.Start(context.Background())

            if tt.wantErr {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
            }
        })
    }
}
```

### Generating Mocks

Use mockery for mock generation:

```bash
# Generate mocks for all modules
make generate

# Or generate for a specific module
cd multinode && mockery
```

Mock configuration is in `.mockery.yaml` files within each module.

## Pull Request Workflow

### Before Submitting

1. **Update tests**: Add or update tests for your changes
2. **Run tests**: Ensure all tests pass locally
3. **Tidy modules**: Run `make gomodtidy`
4. **Generate**: Run `make generate` if interfaces changed
5. **Update docs**: Update documentation if behavior changed

### PR Guidelines

1. **Title**: Use conventional commit format

   - `feat: add new node selector`
   - `fix: handle nil head in tracker`
   - `docs: update architecture diagram`
   - `refactor: simplify transaction lifecycle`

2. **Description**: Include:

   - What changes were made
   - Why the changes were needed
   - How to test the changes
   - Breaking changes (if any)

3. **Size**: Keep PRs focused and reasonably sized
   - Large changes should be split into smaller PRs
   - Refactoring PRs should be separate from feature PRs

### Review Process

1. All PRs require at least one approval
2. CI checks must pass
3. Resolve all review comments
4. Squash commits when merging

## Module Management

### Adding Dependencies

```bash
cd <module>
go get github.com/some/dependency@version
make gomodtidy
```

### Creating a New Module

1. Create directory under appropriate parent
2. Initialize go.mod with proper module path
3. Add to workspace if using go.work
4. Update module graph: `make modgraph`

### Version Updates

When updating dependencies across modules:

```bash
# Update a specific dependency in all modules
gomods exec -- go get github.com/some/dep@latest
make gomodtidy
```

## Common Tasks

### Regenerate Module Graph

```bash
make modgraph
```

This updates `go.md` with the current dependency visualization.

### Clean Generated Files

```bash
make rm-mocked
```

Removes all mockery-generated files.

### Updating Documentation

Documentation lives in `docs/`. To preview locally:

```bash
cd docs
npx docsify-cli serve
```

Visit `http://localhost:3000` to preview.

## Getting Help

- **Issues**: Open a GitHub issue for bugs or feature requests
- **Discussions**: Use GitHub Discussions for questions
- **Slack**: Reach out in #blockchain-integrations (internal)
