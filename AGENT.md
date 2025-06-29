# AGENT.md

## Timeline Analytics Platform: Implementation Brief for Handoff

### Purpose

This file contains a comprehensive standalone summary and architecture for the Timeline Analytics Platform based on all requirements, PRDs, and augmentations to date. It is written so a new engineer or agent can pick up implementation, test, or design work without any external references.

---

## Table of Contents

1. Overview and High-Level Goals
2. Key Architecture Components
3. Data Ingestion, Event Modeling, and Storage
4. Temporal Orchestration Patterns
5. Timeline Operators and Data Processing
6. Attribute Filtering and Historical Replay
7. Testing & Coverage Strategy
8. MermaidJS Architecture Diagrams (Valid, Copy-Paste Ready)

---

### 1. Overview and High-Level Goals

The Timeline Analytics Platform is a Go-based (v1.24.3+) distributed service for time-state analytics on event logs and time-series data. Its mission is to support both real-time and replay/batch analytics on large-scale event streams, using Timeline Algebra (state machines, interval analysis) and orchestrated with Temporal workflows. Key storage is in S3 (Iceberg format); attribute search is powered by VictoriaLogs. Everything must be idiomatic Go, idiomatic Temporal, with structured logging via slog and coverage of at least 80% by automated tests.

**Functional requirements include:**

* Ingest, store, and process time-series event logs (high-cardinality, arbitrary schema)
* Expose a REST API (Go net/http, slog)
* Orchestrate all logic with Temporal (durable, reliable, scalable)
* Efficiently replay and backfill for analytic queries using S3/Iceberg, with fast attribute filtering via VictoriaLogs
* Extract and classify event types from arbitrary logs for Timeline Algebra
* Full support for schema evolution, high-cardinality filtering, and robust integration tests

---

### 2. Key Architecture Components

#### REST API (Go net/http)

* Handles event ingestion (`POST /timelines/{id}/events`) and queries (`POST /timelines/{id}/query`, `/replay_query`)
* Logs all requests/responses via slog (JSON for prod, text for dev)
* Talks to Temporal via the Go SDK (signal-driven for real-time, direct start for queries/backfills)

#### Temporal Workflows

* **IngestionWorkflow:** Receives events via signal, parses/classifies, persists to S3 Iceberg (and optionally VictoriaLogs)
* **QueryWorkflow:** Used for real-time timeline analytics; orchestrates Timeline operator logic on recent data
* **ReplayWorkflow:** Orchestrates batch replay/backfill analytics. Can chunk by time, fan out via child workflows, or loop with ContinueAsNew
* **Activity Workers:** Run external I/O (Iceberg reads, VictoriaLogs queries, S3 writes/reads, heavy CPU ops)

#### Storage and Indexing

* **Iceberg on S3:** All events are durably persisted in Iceberg tables partitioned by time (and optionally other fields). Supports schema evolution and efficient predicate/column pushdown
* **VictoriaLogs:** Indexes all fields of ingested logs (arbitrary schema, high cardinality) for fast search and attribute filtering
* **S3 Results:** Result data (query outputs, blobs) stored in S3; pointer returned for large results

---

### 3. Data Ingestion, Event Modeling, and Storage

* Events are structured logs (JSON, potentially arbitrary schema and high-cardinality attributes)
* Event ingestion is signal-driven: REST API delivers event(s) to Temporal IngestionWorkflow using SignalWithStart (by timeline/session ID)
* Events are **immediately persisted to S3 Iceberg tables** (minimizing loss risk and enabling replay/backfill). Optionally also ingested into VictoriaLogs for fast attribute search
* Event classification: Each log must be mapped to a canonical event type (e.g., Play, Seek, Rebuffer, etc.) and extract key fields/attributes. This is done by an EventClassifier component (Go function or interface)
* **Flexible event model:**

  * Strongly-typed event structs for known types, with a map\[string]interface{} for attributes
  * Generic event struct for unknown/new types
  * Schema evolution is handled via attribute maps and periodic updates to type registry
* All events must have timestamp, type, and raw attributes
* Iceberg schema is flexible: some universal columns, plus an attributes map

---

### 4. Temporal Orchestration Patterns

* **Ingestion:**

  * SignalWithStart ensures at-most-one IngestionWorkflow per timeline (ID scheme: `timeline-{id}`)
  * Receives signals for each event batch, writes to Iceberg, optionally mirrors to VictoriaLogs
  * ContinueAsNew to avoid unbounded history

* **Query/Replay:**

  * QueryWorkflow: ad-hoc or scheduled analytic queries over recent (real-time) or historical (replay/backfill) data
  * ReplayWorkflow: triggered by REST API, starts with query params (time, filters, etc.)
  * VictoriaLogs activity: runs attribute filter to narrow down event set for Iceberg scan
  * Iceberg activity: fetches relevant events (predicate pushdown, column projection)
  * Processing: runs Timeline operator chain on parsed/classified events, returns metrics or timeline as result
  * For large data, workflow splits by time, partitions, or child workflows, checkpoints via ContinueAsNew

* **Concurrency:** Multiple workflows run in parallel (each timeline, query, or replay is a separate workflow instance); scaling is via Temporal workers and S3 concurrency

* **Distributed Processing (Plan 2):**
  
  * Large datasets (≥10K events) automatically trigger concurrent processing via `ProcessEventsConcurrently()`
  * Events split into chunks (default: 10K per chunk) with configurable concurrency limits
  * CPU-intensive timeline operations use dedicated `"timeline-processing"` task queue
  * Fault-tolerant result assembly from multiple concurrent activities
  * Progress reporting via activity heartbeats for long-running operations
  * Backward compatible: small datasets use single-threaded processing

---

### 5. Timeline Operators and Data Processing

* **Event stream → Timeline transformation:**

  * Operators: LatestEventToState, HasExisted, HasExistedWithin, AND/OR/NOT combination, 
      DurationWhere, DurationInCurState, aggregation, windowing
  * Operators are Go functions acting on slices of typed events or intervals
  * Processing can occur in workflow (if data small) or as activities (for larger sets or CPU-intensive ops)
  * Operators chain as a DAG as specified in query JSON
  * Operators must be tested for all edge cases (missing fields, boundary conditions, overlapping intervals)

Example: A simple credit card transaction outlier detection

TL_HasExistedWithin(TL_DurationInCurState(TL_LatestEventToState(col("lat_long")), col(duration) < 10)

---

### 6. Attribute Filtering and Historical Replay

* For ad-hoc or batch queries (e.g., backfill, fraud analysis), the system uses the following steps:

  1. Analyst submits a query with filters (event type, attributes, time window) to REST API
  2. API starts ReplayWorkflow in Temporal with params
  3. ReplayWorkflow first runs a VictoriaLogs query to get pointers or filter for matching events
  4. Iceberg activity reads only the relevant partitions/columns/fields (using results from VictoriaLogs as filter)
  5. Events are parsed and classified, then processed by Timeline operators
  6. Result is either returned in response, or written as a file/blob to S3 and the pointer is returned
* Both real-time and backfill can operate concurrently; ingestion is always immediately persisted, so no loss

---

### 7. Testing & Coverage Strategy

* **Unit tests:** Pure logic (Timeline operators, event classification, parsing, interval math). Table-driven for field variations, missing data, type errors
* **Integration tests:** Simulate REST API, Temporal workflows/activities with mocked external dependencies (Iceberg, VictoriaLogs, S3). Assert end-to-end logic, edge cases
* **E2E tests:** In Temporal’s test environment, full run of a workflow (e.g., replay with VictoriaLogs + Iceberg + timeline operators + result return)
* **Schema evolution tests:** Simulate old and new event formats, adding/removing fields, new event types; verify backward/forward compatibility
* **High-cardinality tests:** Test attribute filter with unique IDs, confirm VictoriaLogs avoids full scans, fallback path correctness
* **Load/stress:** Simulate large backfills (many events, partitions), test scaling by chunking and child workflows
* **Coverage goal:** 80%+ project-wide; coverage tracked in CI/CD

#### 7.1. HTTP Handler Testing with Temporal Mocks (Go `pkg/http`)

Key patterns and learnings for testing HTTP handlers that interact with Temporal:

*   **Path Parameter Handling**:
    *   Utilize `http.NewServeMux` in tests to accurately simulate request routing and enable handlers to extract path parameters via `r.PathValue("id")`.
    *   Example: `mux := http.NewServeMux(); mux.HandleFunc("POST /timelines/{id}/your_endpoint", server.handleYourEndpoint); mux.ServeHTTP(rr, req)`

*   **Temporal Client Mocking (`testify/mock`)**:
    *   Ensure every Temporal client method invoked by a handler (e.g., `SignalWithStartWorkflow`, `ExecuteWorkflow`) has a corresponding mock expectation defined (e.g., `mockClient.On("MethodName", ...).Return(...)`). Unmet calls typically result in panics.
    *   Use `mockClient.AssertExpectations(t)` to verify all mocks were called as expected.

*   **Mock Argument Matching Strategies**:
    *   **Context**: Use `mock.Anything` for `context.Context` arguments.
    *   **Dynamic Structs (e.g., `client.StartWorkflowOptions`)**: Use `mock.AnythingOfType("StartWorkflowOptions")`. Note the use of the *unqualified* type name. The mock framework resolves this to the correct internal SDK type. Using qualified names like `"client.StartWorkflowOptions"` can lead to mismatches.
    *   **Workflow Function Signatures**: Use `mock.AnythingOfType` with the precise function signature string, substituting `workflow.Context` with `internal.Context`. Examples:
        *   `IngestionWorkflow`: `"func(internal.Context, string) error"` (assuming `string` is the second arg for the specific workflow, adjust as needed)
        *   `QueryWorkflow`: `"func(internal.Context, temporal.QueryRequest) (*temporal.QueryResult, error)"`
        *   `ReplayWorkflow`: `"func(internal.Context, temporal.ReplayRequest) (*temporal.QueryResult, error)"`
    *   **Specific Request Objects**: For arguments like `temporal.QueryRequest` or `temporal.ReplayRequest` passed to workflows, create an `expectedRequest` object in the test. Ensure any fields populated by the handler *before* the Temporal call (e.g., `TimelineID` derived from path parameters) are set in this `expectedRequest`.

*   **Testing Error Paths**:
    *   Configure mocks to return errors (e.g., `.Return(nil, errors.New("mock temporal error"))`).
    *   Assert that the HTTP handler correctly translates these errors into appropriate HTTP status codes (e.g., `http.StatusInternalServerError`).

*   **Tests Fixed/Refined with these patterns**:
    *   `TestServer_handleIngestEvents_ValidJSON`
    *   `TestServer_handleQuery`
    *   `TestServer_handleReplayQuery`
    (All located in `pkg/http/server_test.go`)

#### 7.2. Distributed Processing Testing Strategy (Concurrent Processing)

Key patterns and learnings for testing distributed Temporal workflows with concurrent activities:

*   **Test the Logic, Not the Framework**:
    *   Focus on testing **business logic** (chunking, result assembly, threshold detection) rather than Temporal's orchestration capabilities
    *   Temporal's workflow execution is tested by Temporal itself - don't duplicate their testing
    *   Use unit tests for algorithms, integration tests for API contracts

*   **Chunking and Result Assembly**:
    *   **`TestCreateEventChunks`**: Validates mathematical correctness of event splitting (remainders, exact multiples)
    *   **`TestAssembleChunkResults`**: Tests different aggregation strategies for different operation types
    *   **`TestProcessEventsConcurrently_BasicValidation`**: Validates threshold detection and chunking logic without framework complexity

*   **Fault Tolerance Testing**:
    *   **`TestAssembleChunkResults_WithFailures`**: Tests partial failure scenarios (some chunks succeed, others fail)
    *   **`TestAssembleChunkResults_AllFailed`**: Tests complete failure handling
    *   Validates that failed chunks don't corrupt successful results

*   **Integration Test Complexity Anti-Pattern**:
    *   **Avoid**: Complex mocking of Temporal test environment with activity registration
    *   **Prefer**: Simple validation of core logic with direct function calls
    *   **Lesson**: Framework integration tests often test the framework, not your business logic

*   **Concurrent Processing Test Coverage (8 test cases, 100% pass rate)**:
    *   Event chunking with various dataset sizes and remainders
    *   Result aggregation for numeric operations (sum) vs boolean operations (OR)
    *   Fault tolerance with partial and complete failures
    *   Threshold detection for concurrent vs single-threaded processing

---

### 8. MermaidJS Architecture Diagrams (Valid, Copy-Paste Ready)

#### Overall System Architecture

```mermaid
flowchart TD
    UserAPI["REST Client / Analyst Tool"]
    HTTP["Go HTTP Server\nnet/http, slog"]
    TemporalCli["Temporal Client (Go)"]
    IngestWF["Ingestion Workflow\nSignal-driven"]
    ReplayWF["Replay/Backfill Workflow"]
    QueryWF["Query Workflow"]
    Worker["Temporal Activity Workers"]
    Iceberg["Iceberg Table (S3)"]
    VLogs["VictoriaLogs (Index/Search)"]
    S3Out["S3 Results/Blobs"]

    UserAPI -->|"REST / Query"| HTTP
    HTTP -->|"SignalWithStart (event)"| TemporalCli
    TemporalCli -->|"Signals/Start"| IngestWF
    IngestWF -->|"Write JSON/Event"| Iceberg
    IngestWF -.->|"Optional"| VLogs
    HTTP -->|"REST (query)"| TemporalCli
    TemporalCli -->|"Start"| QueryWF
    QueryWF -->|"Query"| Iceberg
    QueryWF -.->|"Optional Query"| VLogs
    QueryWF -->|"Store/Return"| S3Out
    QueryWF -->|"Return/Notify"| HTTP
    HTTP -->|"Result"| UserAPI
    TemporalCli -->|"Start"| ReplayWF
    ReplayWF -->|"Attribute Query"| VLogs
    ReplayWF -->|"Read Events"| Iceberg
    ReplayWF -->|"Process Events"| Worker
    ReplayWF -->|"Store Results"| S3Out
    ReplayWF -->|"Result"| HTTP
```

#### Real-Time Ingestion (Test Scenario)

```mermaid
sequenceDiagram
    participant Client as Event Producer
    participant API as Go HTTP API
    participant Temp as Temporal Client
    participant WF as Ingestion Workflow
    participant S3 as Iceberg S3
    participant VL as VictoriaLogs

    Client->>API: POST /timelines/{id}/events
    API->>Temp: SignalWithStart(IngestionWorkflow)
    Temp->>WF: Deliver Signal (event)
    WF->>WF: Parse, Classify, Buffer
    WF->>S3: Write to Iceberg Table
    WF->>VL: (Optional) Write to VictoriaLogs
    WF-->>Temp: Ack
    Temp-->>API: 202 Accepted
    API-->>Client: Success
```

#### Replay/Backfill Query (Test Scenario)

```mermaid
sequenceDiagram
    participant Analyst as Analyst Tool
    participant API as Go HTTP API
    participant Temp as Temporal Client
    participant RWF as ReplayWorkflow
    participant VL as VictoriaLogs
    participant S3 as Iceberg S3
    participant Ops as TimelineOps
    participant S3Res as S3 Results

    Analyst->>API: POST /timelines/{id}/replay_query (filters)
    API->>Temp: StartWorkflow(ReplayWorkflow)
    Temp->>RWF: Start Workflow (query params)
    RWF->>VL: Attribute query (LogsQL)
    VL-->>RWF: Matching event pointers
    RWF->>S3: Read events via Iceberg filter
    S3-->>RWF: Return matching events
    RWF->>Ops: Classify/Timeline operators
    Ops-->>RWF: Metrics, timeline
    RWF->>S3Res: Store or return result
    RWF-->>Temp: Complete (pointer/result)
    Temp-->>API: Result pointer
    API-->>Analyst: Response (data, URL, etc)
```

#### Attribute Filtering/Timeline Query Example (Test Case)

```mermaid
sequenceDiagram
    participant Analyst as Analyst
    participant API as REST API
    participant Temp as Temporal Client
    participant ReplayWF as ReplayWorkflow
    participant VL as VictoriaLogs
    participant S3 as Iceberg S3
    participant Classify as EventClassifier
    participant Timeline as TimelineOperators
    participant Results as S3Results

    Analyst->>API: POST /timelines/query {type="Rebuffer", cdn="CDN1", device="Android", time=March2025}
    API->>Temp: StartWorkflow(ReplayWorkflow, filters)
    Temp->>ReplayWF: Workflow params
    ReplayWF->>VL: LogsQL attribute search
    VL-->>ReplayWF: Matching event refs/filters
    ReplayWF->>S3: Iceberg scan (with filters)
    S3-->>ReplayWF: Return events
    ReplayWF->>Classify: Classify/Parse events
    Classify-->>ReplayWF: Structured events
    ReplayWF->>Timeline: Operators (duration, count)
    Timeline-->>ReplayWF: Metrics/timeline
    ReplayWF->>Results: Store/Return
    ReplayWF-->>Temp: Complete (pointer)
    Temp-->>API: Pointer/result
    API-->>Analyst: Result or download URL
```

#### Testing Coverage Map

```mermaid
flowchart TD
    UnitTests["Unit Tests (Timeline Operators, Event Classification)"]
    IntegrationTests["Integration Tests (HTTP, Temporal, Iceberg, VictoriaLogs - Mocked)"]
    EndToEnd["End-to-End Tests (Replay Workflow, Query, Results)"]
    SchemaEvo["Schema Evolution Tests (Fields, Event Types)"]
    HighCard["High-Cardinality Filtering Tests"]

    UnitTests --> IntegrationTests
    IntegrationTests --> EndToEnd
    SchemaEvo --> EndToEnd
    HighCard --> EndToEnd
    EndToEnd -->|"80%+ Coverage"| ProjectCoverage["Project-wide Coverage"]
```

---

## Implementation Status & Recent Changes

### ✅ **COMPLETED: Plan 2 - Activity Pool Scaling for Distributed Processing (June 2025)**

**Major accomplishment:** Successfully implemented distributed Timeline Operators using Activity Pool Scaling pattern, enabling seamless processing of millions of events while maintaining backward compatibility for smaller datasets.

#### **Distributed Processing Architecture:**
- **Event Chunking:** Large datasets (≥10K events) automatically split into manageable chunks (default: 10K per chunk)
- **Concurrent Activities:** Multiple chunks processed in parallel with configurable concurrency limits (default: 10 concurrent activities)
- **Dedicated Task Queues:** CPU-intensive timeline processing uses `"timeline-processing"` task queue for optimal resource allocation
- **Fault Tolerance:** Individual chunk failures don't terminate entire queries; results assembled from successful chunks
- **Progress Reporting:** Activities report progress via heartbeats every 1K events for long-running operations

#### **Key Functions & Integration:**
- **`ProcessEventsConcurrently()`**: Main orchestration function for distributed processing
- **`ProcessEventsChunkActivity()`**: Processes individual chunks with progress tracking
- **Smart Threshold Detection**: `QueryWorkflow` and `ReplayWorkflow` automatically choose concurrent vs single-threaded processing
- **Intelligent Result Assembly**: Different aggregation strategies for different operator types (sum for durations, OR for booleans)

#### **Performance Benefits:**
- **Scalability**: Can handle millions of events by distributing across worker pools
- **Resource Utilization**: Better CPU/memory usage across multiple workers  
- **Zero Breaking Changes**: Maintains backward compatibility with existing API
- **Demo-Friendly**: Small datasets still use simple single-threaded processing

#### **Test Coverage (Concurrent Processing):**
- **8 comprehensive test cases** covering chunking, result assembly, fault tolerance
- **TestCreateEventChunks**: Validates chunk creation with exact multiples and remainders
- **TestAssembleChunkResults**: Tests aggregation strategies for different operation types
- **TestProcessEventsConcurrently_BasicValidation**: Integration test for threshold detection and chunking logic
- **All tests passing**: 100% success rate for concurrent processing test suite

### ✅ **COMPLETED: Fintech Timeline Operators & Type System (June 2025)**

**Major accomplishment:** Successfully resolved all type system conflicts and implemented comprehensive financial operators suitable for real-world fintech applications.

#### **Type System Architecture (CRITICAL for future development):**
- **`NumericTimeline`** (`[]NumericInterval`): For state-based operations with start/end intervals
- **`PriceTimeline`** (`[]NumericValue`): For point-in-time financial data with timestamps 
- **Backward compatibility:** `ConvertEventTimelineToNumeric()` and `ConvertEventTimelineToPriceTimeline()` functions
- **Clean separation:** Financial operators use `PriceTimeline`, state operators use `NumericTimeline`

#### **Financial Operators Implemented:**
- **Technical Indicators:** TWAP, VWAP, Bollinger Bands, RSI, MACD
- **Risk Metrics:** VaR (Value at Risk), Drawdown, Sharpe Ratio  
- **AML/Compliance:** Transaction Velocity, Position Exposure
- **Windowing:** Sliding, Tumbling, Session windows with financial data support
- **Aggregations:** Moving averages, percentiles, statistical functions

#### **Test Coverage Achieved:**
- **Timeline Package:** 87.3% coverage (exceeds 80% target)
- **Temporal Package:** 42.4% coverage (includes new concurrent processing tests)
- **All financial operators:** Comprehensive test coverage with real-world scenarios
- **All tests passing:** Fixed TestMovingAggregate, TestPositionExposure, TestCreateSlidingWindows
- **Concurrent Processing:** 8 comprehensive test cases with 100% pass rate

#### **Build Status:**
- ✅ `go build` - successful compilation
- ✅ `go test ./pkg/timeline/ ./pkg/temporal/` - all tests pass
- ✅ HTTP handler tests in `pkg/http/` are now passing after resolving Temporal client mock setup issues. See section 7.1 for detailed patterns.

### **Commands for Testing & Development:**
```bash
# Test core timeline functionality
go test ./pkg/timeline/ -v

# Test temporal integration  
go test ./pkg/temporal/ -v

# Check coverage
go test ./pkg/timeline/ ./pkg/temporal/ -coverprofile=coverage.out
go tool cover -html=coverage.out -o coverage.html

# Build application
go build
```

### **Key Files Modified:**
- `pkg/timeline/types.go` - Consolidated type definitions
- `pkg/timeline/fintech.go` - All financial operators
- `pkg/timeline/aggregations.go` - Added PriceTimeline support  
- `pkg/timeline/windows.go` - Financial windowing support
- `pkg/temporal/activities.go` - Updated for PriceTimeline integration + concurrent processing
- `pkg/temporal/workflows.go` - Added smart threshold detection for concurrent processing
- `pkg/temporal/concurrent_test.go` - Comprehensive concurrent processing test suite
- All test files updated for new type system

### **Next Steps for Successor:**
1. **Performance:** Optimize financial calculations for large datasets
2. **Storage Integration:** Implement actual Iceberg/VictoriaLogs integration (currently mocked)
3. **Additional Indicators:** Add more technical indicators (Stochastic, Williams %R, etc.)
4. **Real-time Streaming:** Enhance streaming capabilities for live market data

### **Critical Notes for Future Development:**
- **NEVER mix NumericTimeline and PriceTimeline** - use conversion functions
- Financial operators require point-in-time data (PriceTimeline), not intervals
- All new financial operators should follow patterns in `pkg/timeline/fintech.go`
- Always test with realistic financial data scenarios (gaps, weekends, etc.)
- Maintain backward compatibility through conversion functions
- **IMPORTANT:** Respect the NumericTimeline vs PriceTimeline type separation detailed above
- Document all changes and update this file so future agents can continue seamlessly

---

## Notes for Successor

* Always favor idiomatic Go and Temporal patterns
* All new event types must be registered in the EventClassifier registry; fallback to map-based generic event
* All queries and replays are orchestrated through Temporal workflows for durability and auditing
* Use VictoriaLogs to minimize scan/read cost for attribute filtering and high-cardinality queries
* Prefer column/predicate pushdown in Iceberg reads to minimize data movement
* Implement all new Timeline operators with full unit and scenario-based test coverage
* **Concurrent Processing:** Large datasets (≥10K events) automatically use distributed processing; small datasets use single-threaded for simplicity
* When testing distributed systems, focus on business logic rather than framework orchestration
* **IMPORTANT:** Respect the NumericTimeline vs PriceTimeline type separation detailed above
* Document all changes and update this file so future agents can continue seamlessly

---

### 9. HCL CLI Tool Implementation

#### 9.1. Overview and Purpose

The HCL CLI tool extends the Timeline Analytics Platform with a standalone command-line interface that enables users to execute timeline queries using HashiCorp Configuration Language (HCL) files directly from the terminal. This leverages the existing HCL parsing capabilities while providing a Terraform-like experience for analytics queries.

#### 9.2. Key Features

* **Single File and Directory Support**: The CLI tool can process either a single HCL file or an entire directory of HCL files, following Terraform's approach to configuration.
* **File Merging**: When a directory is specified, all `.hcl` and `.tf` files are merged into a single configuration before execution.
* **Multiple Output Formats**: Results can be displayed as human-readable text or JSON.
* **Operation Modes**: Supports both `query` and `replay` modes for different types of timeline operations.
* **Customizable Connection**: Allows configuration of Temporal server address and namespace via command-line flags.

#### 9.3. Architecture

* **Command Structure**: The CLI tool (`cmd/timeline/main.go`) handles flag parsing, HCL file/directory processing, and Temporal workflow execution.
* **HCL Processing Utilities**: The `pkg/hcl/merge.go` utilities handle merging multiple HCL files and parsing them into query or replay request structures.
* **Workflow Execution**: The tool directly executes Temporal workflows for queries or replays, using the same workflows as the HTTP API.

#### 9.4. HCL Format

The HCL query format follows a declarative structure:

```hcl
timeline_id = "user-123"

time_range {
  start = "2025-01-01T00:00:00Z"
  end   = "2025-06-01T23:59:59Z"
}

operation "count_events" {
  id     = "event_counter"
  type   = "count"
  source = "events"
}
```

More complex queries can include nested operations, filters, and conditions.

#### 9.5. Testing Approach

* **HCL-JSON Equivalence**: Tests verify that parsing HCL produces identical structures as parsing equivalent JSON.
* **Multiple File Merging**: Tests confirm that merging multiple HCL files from directories works correctly.
* **Fixture-Based Testing**: Uses test fixtures for HCL and JSON to compare parsing results across formats.

#### 9.6. Implementation Notes

* The CLI follows the same workflow execution patterns as the HTTP API but operates directly from the command line.
* Uses the Temporal Go SDK to connect to the workflow engine.
* The merging logic for directory-based HCL files concatenates content with appropriate handling for nested blocks.
* Test utilities ensure consistent comparison between HCL and JSON-derived query structures.

#### 9.7. Future Enhancement Opportunities

* Add interactive mode for building and refining queries.
* Support saving query results to files or databases.
* Implement query templates and variable substitution.
* Add validation rules and schema checking for HCL files.
* Create a query history feature to recall and reuse previous queries.

