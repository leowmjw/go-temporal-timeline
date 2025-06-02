# Timeline Framework Service

A Go-based distributed service for time-state analytics on event logs and time-series data, implementing Timeline Algebra concepts with Temporal workflow orchestration.

## Overview

The Timeline Framework enables declarative analytics on temporal event data using timeline-based abstractions instead of traditional tabular data processing. This service provides:

- **Timeline Algebra Operations**: LatestEventToState, HasExisted, DurationWhere, logical combinations (AND/OR/NOT)
- **Event Classification**: Automatic parsing and classification of JSON event logs into strongly-typed events
- **Temporal Orchestration**: Reliable, durable workflows for both real-time and replay/backfill analytics
- **Storage Integration**: S3/Iceberg for durable storage, VictoriaLogs for fast attribute filtering
- **REST API**: Simple HTTP interface for event ingestion and timeline queries

## Architecture

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   REST API      │    │   Temporal      │    │   Storage       │
│   (Go net/http) │────│   Workflows     │────│   S3/Iceberg    │
│                 │    │                 │    │   VictoriaLogs  │
└─────────────────┘    └─────────────────┘    └─────────────────┘
         │                       │                       │
         │                       │                       │
    Event Ingestion         Orchestration          Durable Storage
         │                       │                       │
         ▼                       ▼                       ▼
   ┌──────────┐           ┌──────────┐           ┌──────────┐
   │ Timeline │           │ Query/   │           │ Event    │
   │ Operators│           │ Replay   │           │ Storage  │
   │          │           │ Logic    │           │          │
   └──────────┘           └──────────┘           └──────────┘
```

## Key Components

### 1. Timeline Library (`pkg/timeline/`)
- **Event Classification**: Automatic parsing of JSON logs into typed events (Play, Seek, Rebuffer, etc.)
- **Timeline Operators**: Core algebra operations for temporal analysis
- **Type System**: StateTimeline, BoolTimeline, EventTimeline with full test coverage

### 2. Temporal Integration (`pkg/temporal/`)
- **IngestionWorkflow**: Signal-driven event processing with durable persistence
- **QueryWorkflow**: Real-time timeline analytics execution
- **ReplayWorkflow**: Historical/backfill analytics with VictoriaLogs filtering
- **Activities**: Storage operations, event processing, and external integrations

### 3. HTTP API (`pkg/http/`)
- **Event Ingestion**: `POST /timelines/{id}/events`
- **Real-time Query**: `POST /timelines/{id}/query`
- **Replay Query**: `POST /timelines/{id}/replay_query`
- **Health Check**: `GET /health`

## Quick Start

### Prerequisites
- Go 1.24.3+
- Temporal server (local or remote)

### Build and Run

```bash
# Build the server
make build

# Run tests
make test

# Start the server (requires Temporal server running)
make server

# Or run with custom configuration
go run ./cmd/server -http-addr :8080 -temporal-addr localhost:7233
```

### Basic Usage

#### 1. Ingest Events
```bash
curl -X POST http://localhost:8080/timelines/video123/events \
  -H "Content-Type: application/json" \
  -d '[
    {"eventType":"play","timestamp":"2025-01-01T12:00:00Z","video_id":"video123"},
    {"eventType":"playerStateChange","timestamp":"2025-01-01T12:01:00Z","state":"buffer"},
    {"eventType":"playerStateChange","timestamp":"2025-01-01T12:02:00Z","state":"play"}
  ]'
```

#### 2. Execute Timeline Query
```bash
curl -X POST http://localhost:8080/timelines/video123/query \
  -H "Content-Type: application/json" \
  -d '{
    "operations": [
      {
        "id": "bufferPeriods",
        "op": "LatestEventToState",
        "source": "playerStateChange",
        "equals": "buffer"
      },
      {
        "id": "result",
        "op": "DurationWhere",
        "conditionAll": ["bufferPeriods"]
      }
    ]
  }'
```

## Timeline Algebra Examples

### Connection-Induced Rebuffering (CIR)
Calculate rebuffering duration caused by connection issues, excluding user-initiated seeks:

```json
{
  "operations": [
    {
      "id": "bufferPeriods",
      "op": "LatestEventToState",
      "source": "playerStateChange",
      "equals": "buffer"
    },
    {
      "id": "afterPlay",
      "op": "HasExisted",
      "source": "playerStateChange",
      "equals": "play"
    },
    {
      "id": "noRecentSeek",
      "op": "Not",
      "of": {
        "op": "HasExistedWithin",
        "source": "userAction",
        "equals": "seek",
        "window": "5s"
      }
    },
    {
      "id": "result",
      "op": "DurationWhere",
      "conditionAll": ["bufferPeriods", "afterPlay", "noRecentSeek"]
    }
  ]
}
```

### CDN Performance Analysis
Analyze rebuffering by CDN with attribute filtering:

```bash
curl -X POST http://localhost:8080/timelines/video123/replay_query \
  -H "Content-Type: application/json" \
  -d '{
    "query": {
      "operations": [
        {
          "id": "result",
          "op": "DurationWhere",
          "source": "rebuffer"
        }
      ],
      "filters": {"cdn": "CDN1", "device_type": "Android"},
      "time_range": {
        "start": "2025-01-01T00:00:00Z",
        "end": "2025-01-31T23:59:59Z"
      }
    }
  }'
```

## Event Types

The system supports automatic classification of these event types:

- **PlayEvent**: Video playback start (`video_id`, `cdn`, `device_type`)
- **SeekEvent**: User seeking (`seek_from_time`, `seek_to_time`, `video_id`)
- **RebufferEvent**: Rebuffering occurrences (`buffer_duration`, `cdn`, `device_type`)
- **PlayerStateChange**: State transitions (`state`: play/pause/buffer)
- **CDNChange**: CDN switches (`cdn`)
- **UserAction**: User interactions (`action`)
- **GenericEvent**: Fallback for unknown event types

Custom event types can be registered via the EventClassifier.

## Testing

```bash
# Run core tests
make test

# Run all tests (including complex integrations)
make test-all

# Generate coverage report
make coverage

# Code quality checks
make check
```

## Configuration

Environment variables and command-line flags:

- `--http-addr`: HTTP server address (default: `:8080`)
- `--temporal-addr`: Temporal server address (default: `localhost:7233`)
- `--namespace`: Temporal namespace (default: `default`)
- `--task-queue`: Temporal task queue (default: `timeline-task-queue`)
- `--log-level`: Log level (default: `info`)

## Development

### Project Structure
```
├── cmd/server/          # Main application
├── pkg/
│   ├── timeline/        # Core timeline algebra and event classification
│   ├── temporal/        # Workflow and activity definitions
│   └── http/           # REST API server
├── test/               # Integration and E2E tests
└── internal/           # Internal utilities
```

### Key Design Principles

1. **Timeline Algebra**: All analytics operations are expressed as timeline transformations
2. **Event-Driven**: Signal-based ingestion with workflow orchestration
3. **Durability**: Temporal ensures reliable execution, S3/Iceberg ensures data persistence
4. **Schema Evolution**: Support for changing event schemas over time
5. **High Cardinality**: VictoriaLogs enables efficient filtering on arbitrary attributes

## Contributing

1. Ensure all tests pass: `make test`
2. Follow Go conventions and existing code patterns
3. Add tests for new functionality (target 80%+ coverage)
4. Update documentation for new features

## License

MIT License - see [LICENSE](LICENSE) file for details.

## Related Work

This implementation is based on concepts from:
- **Timeline Framework**: Time-state analytics via timeline abstractions
- **Temporal**: Workflow orchestration for reliable distributed systems
- **Apache Iceberg**: Open table format for large analytic datasets
- **VictoriaLogs**: Fast log search and analytics engine