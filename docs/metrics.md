# Prometheus Metrics

## Overview

This document defines all Prometheus metrics exposed by the email validation system. All services expose metrics on port `:9090/metrics` by default.

## Metric Naming Convention

All metrics follow the pattern: `email_validator_{component}_{metric_name}_{unit}`

- **Component**: `api`, `worker`, `orchestrator`, `verifier`
- **Metric Name**: Descriptive name in snake_case
- **Unit**: `_total` (counter), `_seconds` (histogram/gauge), `_bytes`, `_ratio`, etc.

---

## API Service Metrics

### Request Metrics

```prometheus
# Total HTTP requests
email_validator_api_requests_total{method="POST", endpoint="/v1/validate", status="200"}

# Request duration histogram
email_validator_api_request_duration_seconds{method="POST", endpoint="/v1/validate"}

# Request size histogram  
email_validator_api_request_size_bytes{method="POST", endpoint="/v1/validate"}

# Response size histogram
email_validator_api_response_size_bytes{method="POST", endpoint="/v1/validate"}

# Active requests gauge
email_validator_api_requests_active{endpoint="/v1/validate"}
```

### Validation Metrics

```prometheus
# Total validations performed
email_validator_api_validations_total{status="valid|invalid|catch-all|unknown|risky"}

# Cache hit/miss
email_validator_api_cache_requests_total{result="hit|miss"}

# Cache hit rate (computed from above)
rate(email_validator_api_cache_requests_total{result="hit"}[5m]) / 
rate(email_validator_api_cache_requests_total[5m])
```

### Rate Limiting Metrics

```prometheus
# Rate limit hits
email_validator_api_rate_limit_exceeded_total{customer_id="...", tier="free|standard|enterprise"}

# Current rate limit usage
email_validator_api_rate_limit_current{customer_id="...", window="hour"}
```

---

## Worker Pool Metrics

### Worker Metrics

```prometheus
# Total workers
email_validator_workers_total

# Active workers
email_validator_workers_active

# Idle workers
email_validator_workers_idle

# Worker utilization (0-1)
email_validator_workers_utilization_ratio
```

### Job Processing Metrics

```prometheus
# Jobs processed
email_validator_worker_jobs_processed_total{status="success|failure|retry"}

# Job processing duration
email_validator_worker_job_duration_seconds

# Jobs currently processing
email_validator_worker_jobs_active
```

---

## SMTP Verifier Metrics

### SMTP Connection Metrics

```prometheus
# SMTP connections total
email_validator_smtp_connections_total{mx_host="...", result="success|failure|timeout"}

# SMTP connection duration
email_validator_smtp_connection_duration_seconds{mx_host="..."}

# Active SMTP connections
email_validator_smtp_connections_active{mx_host="..."}

# SMTP connection pool size
email_validator_smtp_connection_pool_size{mx_host="..."}
```

### SMTP Response Metrics

```prometheus
# SMTP response codes
email_validator_smtp_responses_total{code="250|550|450|421|..."}

# SMTP errors
email_validator_smtp_errors_total{type="timeout|connection_refused|protocol_error|..."}
```

### Validation Metrics

```prometheus
# Validation duration (end-to-end)
email_validator_validation_duration_seconds{status="valid|invalid|catch-all|unknown|risky"}

# Validation results
email_validator_validations_total{status="valid|invalid|catch-all|unknown|risky"}

# Validation confidence distribution
email_validator_validation_confidence_ratio{status="valid|invalid|..."}
```

### Catch-all Detection Metrics

```prometheus
# Catch-all detections
email_validator_catchall_detected_total{domain="..."}

# Catch-all detection duration
email_validator_catchall_detection_duration_seconds
```

---

## DNS/MX Lookup Metrics

```prometheus
# DNS lookups
email_validator_dns_lookups_total{result="success|failure|cached"}

# DNS lookup duration
email_validator_dns_lookup_duration_seconds

# MX records found
email_validator_mx_records_found{domain="...", count="0|1|2|3+"}
```

---

## Queue Metrics

```prometheus
# Queue depth (current size)
email_validator_queue_depth{priority="express|standard|bulk"}

# Messages enqueued
email_validator_queue_messages_enqueued_total{priority="express|standard|bulk"}

# Messages dequeued
email_validator_queue_messages_dequeued_total{priority="express|standard|bulk"}

# Message processing lag (time in queue)
email_validator_queue_message_lag_seconds{priority="express|standard|bulk"}

# Dead letter queue size
email_validator_queue_dlq_size
```

---

## Cache Metrics

### Redis Metrics

```prometheus
# Redis operations
email_validator_redis_operations_total{operation="get|set|del|incr", result="success|failure"}

# Redis operation duration
email_validator_redis_operation_duration_seconds{operation="get|set|del"}

# Redis connection pool
email_validator_redis_connections_active
email_validator_redis_connections_idle

# Memory usage
email_validator_redis_memory_used_bytes
email_validator_redis_memory_max_bytes
```

### Cache Hit Rate Metrics

```prometheus
# MX cache hit rate
email_validator_cache_mx_requests_total{result="hit|miss"}

# Validation result cache hit rate
email_validator_cache_validation_requests_total{result="hit|miss"}

# Domain metadata cache hit rate
email_validator_cache_domain_requests_total{result="hit|miss"}
```

---

## Database Metrics

```prometheus
# Database queries
email_validator_db_queries_total{operation="select|insert|update|delete", table="..."}

# Query duration
email_validator_db_query_duration_seconds{operation="...", table="..."}

# Connection pool
email_validator_db_connections_active
email_validator_db_connections_idle
email_validator_db_connections_wait_duration_seconds

# Batch insert metrics
email_validator_db_batch_inserts_total
email_validator_db_batch_insert_size{table="validation_results"}
```

---

## Domain Statistics

```prometheus
# Validations per domain
email_validator_domain_validations_total{domain="...", status="valid|invalid|..."}

# Top domains (cardinality limited)
email_validator_top_domains_validations_total{domain="gmail.com|yahoo.com|..."}
```

---

## Error Metrics

```prometheus
# Application errors
email_validator_errors_total{component="api|worker|verifier", type="validation|network|database"}

# Panic recovery
email_validator_panics_recovered_total{component="..."}
```

---

## Health Check Metrics

```prometheus
# Component health
email_validator_health_status{component="database|redis|queue", status="healthy|unhealthy"}

# Health check duration
email_validator_health_check_duration_seconds{component="..."}
```

---

## Business Metrics

```prometheus
# Customer usage
email_validator_customer_validations_total{customer_id="...", tier="free|standard|enterprise"}

# Revenue metrics (if applicable)
email_validator_customer_credits_used_total{customer_id="..."}
```

---

## Histogram Buckets

### Latency Buckets (seconds)

```yaml
buckets: [0.1, 0.25, 0.5, 1.0, 2.0, 3.0, 5.0, 10.0, 30.0]
```

Used for:
- `email_validator_validation_duration_seconds`
- `email_validator_api_request_duration_seconds`
- `email_validator_smtp_connection_duration_seconds`

### Size Buckets (bytes)

```yaml
buckets: [100, 1000, 10000, 100000, 1000000]
```

Used for:
- `email_validator_api_request_size_bytes`
- `email_validator_api_response_size_bytes`

---

## Example PromQL Queries

### Performance

```promql
# P95 validation latency
histogram_quantile(0.95, 
  rate(email_validator_validation_duration_seconds_bucket[5m])
)

# P99 API latency
histogram_quantile(0.99, 
  rate(email_validator_api_request_duration_seconds_bucket[5m])
)

# Requests per second
rate(email_validator_api_requests_total[1m])
```

### Cache Performance

```promql
# Overall cache hit rate
sum(rate(email_validator_cache_validation_requests_total{result="hit"}[5m])) /
sum(rate(email_validator_cache_validation_requests_total[5m]))

# MX cache hit rate
rate(email_validator_cache_mx_requests_total{result="hit"}[5m]) /
rate(email_validator_cache_mx_requests_total[5m])
```

### Error Rates

```promql
# Error rate percentage
sum(rate(email_validator_api_requests_total{status=~"5.."}[5m])) /
sum(rate(email_validator_api_requests_total[5m])) * 100

# SMTP timeout rate
rate(email_validator_smtp_errors_total{type="timeout"}[5m])
```

### Worker Utilization

```promql
# Worker utilization percentage
email_validator_workers_active / email_validator_workers_total * 100

# Queue depth
email_validator_queue_depth
```

### Status Distribution

```promql
# Validation status breakdown
sum by (status) (rate(email_validator_validations_total[5m]))
```

---

## Alert Rules

See also: [Alerting Rules](#alerting-rules-section)

### Critical Alerts

```yaml
# High error rate
- alert: HighErrorRate
  expr: |
    (sum(rate(email_validator_api_requests_total{status=~"5.."}[5m])) /
     sum(rate(email_validator_api_requests_total[5m]))) > 0.05
  for: 5m
  labels:
    severity: critical
  annotations:
    summary: "High error rate (> 5%)"

# High latency
- alert: HighLatency
  expr: |
    histogram_quantile(0.95,
      rate(email_validator_validation_duration_seconds_bucket[5m])
    ) > 5
  for: 5m
  labels:
    severity: warning
  annotations:
    summary: "P95 latency > 5s"

# Queue depth high
- alert: QueueDepthHigh
  expr: email_validator_queue_depth > 10000
  for: 5m
  labels:
    severity: warning
  annotations:
    summary: "Queue depth > 10,000"

# Worker pool exhausted
- alert: WorkerPoolExhausted
  expr: email_validator_workers_utilization_ratio > 0.9
  for: 5m
  labels:
    severity: warning
  annotations:
    summary: "Worker utilization > 90%"

# Database connection pool exhausted
- alert: DBConnectionPoolExhausted
  expr: |
    email_validator_db_connections_active /
    (email_validator_db_connections_active + email_validator_db_connections_idle) > 0.9
  for: 5m
  labels:
    severity: critical
  annotations:
    summary: "Database connection pool > 90% utilized"

# Low cache hit rate
- alert: LowCacheHitRate
  expr: |
    sum(rate(email_validator_cache_validation_requests_total{result="hit"}[15m])) /
    sum(rate(email_validator_cache_validation_requests_total[15m])) < 0.5
  for: 15m
  labels:
    severity: warning
  annotations:
    summary: "Cache hit rate < 50%"
```

---

## Grafana Dashboard Panels

Key panels to include:

1. **Overview Row**
   - Validations/sec (gauge)
   - P95 latency (gauge)
   - Error rate (gauge)
   - Cache hit rate (gauge)

2. **Performance Row**
   - Request latency histogram
   - Throughput graph (requests/sec over time)
   - Status distribution (pie chart)

3. **Worker Pool Row**
   - Active workers (gauge)
   - Worker utilization (gauge)
   - Queue depth (graph)

4. **SMTP Row**
   - SMTP connections/sec
   - SMTP response codes distribution
   - Top MX hosts by volume

5. **Cache Row**
   - Cache hit rate by type
   - Redis memory usage
   - Cache operations/sec

6. **Database Row**
   - Query latency
   - Connections (active/idle)
   - Batch insert rate

7. **Errors Row**
   - Error rate over time
   - Error breakdown by type
   - SMTP timeouts

---

## Metric Cardinality Management

### High Cardinality Labels to Avoid

❌ Don't use:
- `email` - millions of unique values
- `customer_id` (without limits) - thousands of customers
- `mx_host` (unlimited) - thousands of MX hosts

✅ Do use:
- `status` - 5 values (valid, invalid, catch-all, unknown, risky)
- `priority` - 3 values (express, standard, bulk)
- `tier` - 3 values (free, standard, enterprise)
- Top N domains only (`domain` label limited to top 100)

### Recommended Limits

- Max unique label values per metric: < 1000
- Total active time series: < 100,000
- Metric retention: 15 days (configurable)

---

## Related Documents

- [architecture.md](file:///Users/bigwolf/.gemini/antigravity/scratch/mail_sorter/docs/architecture.md) - System architecture
- [grafana-dashboard.json](file:///Users/bigwolf/.gemini/antigravity/scratch/mail_sorter/deploy/grafana-dashboard.json) - Grafana dashboard configuration
- [runbook.md](file:///Users/bigwolf/.gemini/antigravity/scratch/mail_sorter/docs/runbook.md) - Operational runbook
