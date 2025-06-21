# Query operations

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
