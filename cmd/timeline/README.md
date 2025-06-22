# Timeline CLI

A command-line tool for executing timeline queries using HCL configuration files.

## Overview

The Timeline CLI allows you to run timeline queries directly from the command line using HCL configuration files, similar to how Terraform processes .tf files. The tool can process either a single HCL file or an entire directory of HCL files.

## Usage

```bash
# Build the CLI tool
cd /path/to/go-temporal-timeline
go build -o timeline ./cmd/timeline

# Run a query using a single HCL file
./timeline -path examples/hcl-queries/simple_query.hcl

# Run a query using a directory of HCL files
./timeline -path examples/hcl-queries/

# Run a query and display results as JSON
./timeline -path examples/hcl-queries/complex_query.hcl -json

# Run a replay query instead of a standard query
./timeline -path examples/hcl-queries/replay_query.hcl -mode replay

# Connect to a specific Temporal server
./timeline -path examples/hcl-queries/simple_query.hcl -address temporal.example.com:7233 -namespace custom
```

## Command-line Options

| Option | Description | Default |
|--------|-------------|---------|
| `-path` | Path to HCL file or directory (required) | |
| `-address` | Address of Temporal server | localhost:7233 |
| `-namespace` | Temporal namespace | default |
| `-json` | Display results as JSON | false |
| `-mode` | Operation mode: 'query' or 'replay' | query |

## HCL File Format

### Simple Query Example

```hcl
# Simple count query
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

### Complex Query Example

```hcl
# Complex query with nested operations
timeline_id = "user-123"

filters = {
  user_id = "123"
  status  = "active"
}

time_range {
  start = "2025-01-01T00:00:00Z"
  end   = "2025-06-01T23:59:59Z"
}

operation "avg_response" {
  id     = "avg_response_time"
  type   = "average"
  window = "5m"
  
  of "nested" {
    id     = "response_times"
    type   = "extract"
    source = "response_time"
  }
}
```

## Multiple File Processing

When processing a directory, the CLI tool will merge all .hcl and .tf files in the directory, allowing you to split your configuration across multiple files like Terraform:

```
examples/hcl-queries/
├── timeline.hcl    # Basic timeline configuration
├── operations.hcl  # Query operations
└── filters.hcl     # Filters and time ranges
```

## Note

Make sure your Temporal server is running before executing queries.
