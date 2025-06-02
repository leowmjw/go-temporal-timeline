# Timeline Framework Service PRD (Go 1.24.3)

## Introduction & Background

The Timeline Framework enables time-state analytics via timeline-based abstractions instead of traditional tabular data processing. This PRD defines a RESTful Go service implementing the Timeline Framework, orchestrated via Temporal.

### Key Components

* **Go (v1.24.3)**: Main language
* **Temporal**: Workflow orchestration
* **S3**: Durable event storage
* **slog**: Structured logging

## Objectives and Key Requirements

### Functional Requirements

* REST API to ingest events and execute timeline queries
* Temporal workflows for ingestion and query orchestration
* Timeline Algebra operations (LatestEventToState, HasExisted, DurationWhere, etc.)
* DAG-based query execution

### Temporal Workflow Strategy

* Ingestion workflow per timeline (ID: `timeline-{entityID}`)
* Query workflows (ID: `query-{timelineID}-{uuid}`)
* Use signals for event ingestion
* Use child workflows for parallel queries (future enhancement)

### Timeline Data Model

* **EventTimeline**: List of events
* **StateTimeline**: Intervals of state
* **BoolTimeline**: Boolean intervals
* **NumericTimeline**: Piecewise numeric values

## API Design

### Endpoints

* `POST /timelines/{id}/events`

  * Ingests one or more events
  * Signals corresponding Temporal workflow

* `POST /timelines/{id}/query`

  * Executes a timeline query
  * Supports synchronous execution (initially)

### Query Spec Example

```json
{
  "operations": [
    { "id": "bufferPeriods", "op": "LatestEventToState", "source": "playerStateChange", "equals": "buffer" },
    { "id": "afterPlay", "op": "HasExisted", "source": "playerStateChange", "equals": "play" },
    { "id": "noRecentSeek", "op": "Not", "of": { "op": "HasExistedWithin", "source": "userAction", "equals": "seek", "window": "5s" } },
    { "id": "cdnFilter", "op": "LatestEventToState", "source": "cdnChange", "equals": "CDN1" },
    { "id": "result", "op": "DurationWhere", "conditionAll": ["bufferPeriods", "afterPlay", "noRecentSeek", "cdnFilter"] }
  ]
}
```

### Response

* JSON result (e.g., `{ "result": 12.5, "unit": "seconds" }`)
* Uses structured logging via `slog`

## Storage & Performance

* Use S3 to persist large event data and timeline snapshots
* Ingestion workflows append to S3 via `AppendEventActivity`
* Use `ContinueAsNew` for ingestion workflows after N events
* Partition data by timeline ID and time

## Query Execution Workflow

1. **Start QueryWorkflow**
2. **Load Events** from S3
3. **Compute Timeline** operations
4. **Return Result** or write to S3 if large

## Testing Strategy

### Unit Testing

* Pure Go tests for timeline operators (LatestEventToState, etc.)
* Table-driven tests for edge cases

### Integration Tests

* Temporal mocks for activities
* Fake S3 client interface

### End-to-End Testing

* Full ingestion + query test with Temporal test suite
* Optional: HTTP tests using Go's test client

## Internal Go Types

```go
type Event struct {
  Timestamp time.Time
  Type string
  Value string
}
type StateInterval struct {
  State string
  Start time.Time
  End time.Time
}
type StateTimeline []StateInterval
```

## Timeline Operator Functions

* `LatestEventToState(events EventTimeline) StateTimeline`
* `HasExisted(events, condition) BoolTimeline`
* `HasExistedWithin(events, condition, window) BoolTimeline`
* `DurationWhere(condition BoolTimeline) time.Duration`
* `AND/OR/NOT` combinations via interval operations

## Temporal Usage Patterns

* `SignalWithStart` for ingestion
* Workflow ID conventions: `timeline-{id}`, `query-{id}-{uuid}`
* Child workflows for scalable parallel query
* Ensure deterministic workflow logic using `workflow.Now()`

## Development Phases

1. Build timeline library (pure logic)
2. Integrate with Temporal (ingestion & query workflows)
3. S3 integration
4. End-to-end testing
5. Documentation and examples

## Conclusion

This design outlines a robust, maintainable Go service for time-state analytics. It abstracts away complexity using the Timeline Framework and Temporal workflows, making complex metrics like CIR (Connection-Induced Rebuffering) declarative and scalable.

Engineers can begin implementation directly based on this document, with full alignment to the CIDR’23 paper concepts.

