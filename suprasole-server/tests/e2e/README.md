# Suprasole Server Integration & End-to-End Test Workspace

Welcome to the suprasole-server E2E integration test workspace. This workspace is designed to validate the end-to-end integration behaviors of the suprasole WebSocket PTY server under strict, declarative constraints.

## 1. Runtime Orchestrator Engine
The integration test suite executes through Deno's native automation framework using the deno test runner:
```bash
./run_tests.sh
```

### Execution Boundary Inclusion
Deno's test runner targets test execution modules located within:
```text
source/**/*.e2e.ts
```

### Isolation Workflow Infrastructure
To prevent test cross-contamination, the orchestrator layer dynamically spawns unique background instances of the compiled Go server binary on free loopback TCP ports (resolved via ephemeral binding). Each execution path runs over isolated connection sockets, ensuring absolute environment isolation and deterministic cleanups upon termination.

## 2. Directory Architecture
The workspace is organized into a strict structural layout:
```text
tests/e2e/
├── deno.json        # Deno config and import mappings
├── run_tests.sh     # Shell script to build and execute tests
├── main.go          # Go entry point compiled during test run
├── README.md        # This workspace description
└── source/
    ├── ServerOrchestrator.ts # Process spawning and lifecycle manager
    ├── WebSocketClient.ts    # WebSocket binary client helper
    └── server.e2e.ts        # The actual E2E test assertions
```

## 3. Black-Box Testing Philosophy & Boundary Opacity
All E2E tests strictly adhere to the black-box testing paradigm:
- **Input-Output Isolation**: All test blocks act exclusively as end-user WebSocket contract consumers. Test routines are strictly forbidden from inspecting internal memory variables, registry maps, or calling internal Go packages directly.
- **Pure Payload Interception**: Assertions operate purely by passing serialized binary frames over the connection and validating responses.
- **Implementation Blindness**: Titles, invariants, and descriptions are entirely opaque to low-level implementation details.

## 4. Mandatory Test Label Format Requirement
Every test case declaration across the E2E test workspace must strictly utilize the Suprasole Tokenized Labeling Standard:
```text
{<Identifier>} [<Scope-Tag>] <Component/Topology> (<Action/State>): <Behavioral Invariant Details>
```
Example:
```typescript
Deno.test({
  name: '{wsg01a} [WebSocket Gateway] Connection Handshake (Connection): Connects and verifies WebSocket protocol upgraded successfully',
  ...
})
```
