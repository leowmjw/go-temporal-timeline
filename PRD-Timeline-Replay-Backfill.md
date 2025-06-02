# Timeline Framework PRD Update: Replay & Backfill Support for Analytics Queries

## Introduction & Background

The Timeline Framework was originally designed for real-time processing of structured JSON event logs. However, use cases have shifted towards retrospective, historical data analysis. This update introduces support for backfill and replay workflows using:

* **Apache Iceberg on S3**: Durable storage
* **Temporal**: Workflow orchestration (both real-time and batch)
* **VictoriaLogs**: Indexing and filtering

### Assumptions & Context

* Events are written to Iceberg tables on S3.
* Temporal handles ingestion, replays, and backfills.
* VictoriaLogs indexes all log fields for fast search.
* Real-time ingestion remains unchanged; analytics queries now trigger replay workflows.

## Architecture for Replay & Backfill Workflows

### Components

* **Durable Storage (Iceberg)**: Partitioned tables with column projection.
* **Indexing (VictoriaLogs)**: Efficient field-based search and query filtering.
* **Temporal Orchestrator**: New `ReplayWorkflow` type coordinates:

  * Receives query parameters
  * Queries VictoriaLogs (optional)
  * Reads filtered events from Iceberg
  * Processes them through Timeline operators
  * Returns or stores results

### Workflow Details

* Workflows use predicate pushdown and column projection in Iceberg.
* Supports checkpointing and continuation via `ContinueAsNew`.
* Real-time and replay are isolated via separate workflows.

## Event Type Extraction & Classification

### Strategy

* Use fields like `eventType` or `action` to classify logs.
* Extract relevant fields per event type:

  * Play: `video_id`, `cdn`, `device_type`, etc.
  * Seek: `seek_from_time`, `seek_to_time`, etc.
  * Rebuffer: `buffer_duration`, `cdn`, `device_type`, etc.
* JSON parsing is implemented in Go.

### Iceberg Table Design

* Schema supports evolution:

  * Wide schema or
  * Key-value attribute map

### EventClassifier

* Takes raw JSON → `(eventType, EventDataStruct)`
* Uses either:

  * Strongly-typed event structs
  * Generic attribute maps
* Hybrid model: structured for known events, generic for others

## Modeling Dynamic Events

### Design Patterns

* `TimelineEvent` interface with methods like `Type()` and `Timestamp()`
* Use attribute maps for extensibility
* Event type registry maps string → parser function
* Schema evolution support using Iceberg features

## Temporal Workflow Design

### Real-Time Ingestion

* Long-running workflow
* Receives events via signals
* Uses `ContinueAsNew` for truncating history

### Replay/Backfill Workflow

* Short-lived, triggered via API/UI/CLI/Schedule
* Uses activities for:

  * `QueryVictoriaLogsActivity`
  * `ReadIcebergActivity`
  * Optional `ProcessEventsActivity`
* Uses checkpointing and retries

### Query Types

* By time range (e.g., March 2025)
* By attribute (e.g., `cdn="CDN1"`)
* Supports partial backfills

## End-to-End Query Flow

### Example: Analyst Query

* Query: All `Rebuffer` events in March 2025, `cdn="CDN1"`, `device="Android"`

### Steps

1. **Trigger ReplayWorkflow** with query params
2. **Query VictoriaLogs** (if available)
3. **Read from Iceberg** using filters and projections
4. **Classify Events** into structs
5. **Process via Timeline Operators**
6. **Return/Store Results**

### Result Types

* Direct return (small results)
* Output to S3 or database (large results)

## Testing Strategy

### Unit Testing

* Temporal's Go SDK (`WorkflowTestSuite`)
* EventClassifier tests with varied JSON
* Timeline operator logic as pure functions

### Integration Testing

* Local Iceberg environment with mock data
* VictoriaLogs mock or local instance
* Full workflow simulation with mocked activities

### Schema Evolution Tests

* Old vs. new fields
* Add new event types
* Replay to test reclassification

### High-Cardinality Filtering

* Test indexing vs. full scan
* Simulate scenarios with unique `user_id`

### Load/Stress Testing

* Simulate full-month backfills
* Concurrent workflow execution

### Idiomatic Go/Temporal Practices

* Separation of concerns
* Deterministic workflow logic
* Use of signals, retries, `ContinueAsNew`

## Conclusion

This update allows Timeline to support batch analytics queries over historical data, using Iceberg for durable storage, VictoriaLogs for indexing, and Temporal for orchestration. With structured event modeling, idiomatic Go design, and robust test coverage, analysts can confidently run historical queries with the same reliability as real-time processing.

