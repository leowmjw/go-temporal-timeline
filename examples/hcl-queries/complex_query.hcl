# Complex query with nested operations and filters
timeline_id = "user-123"

# Filters to narrow results
filters = {
  user_id = "123"
  status  = "active"
}

# Time range for the query
time_range {
  start = "2025-01-01T00:00:00Z"
  end   = "2025-06-01T23:59:59Z"
}

# First operation - counting events
operation "count_events" {
  id     = "event_counter"
  type   = "count"
  source = "events"
}

# Second operation - calculating average response time
operation "avg_response" {
  id     = "avg_response_time"
  type   = "average"
  window = "5m"
  
  # Nested operation with required label
  of "nested" {
    id     = "response_times"
    type   = "extract"
    source = "response_time"
  }
}

# Third operation - maximum value
operation "max_value" {
  id   = "max_cpu"
  type = "max"
  
  of "nested" {
    id     = "cpu_values" 
    type   = "extract"
    source = "resource.cpu"
  }
}
