# Email Deliverability Validation System - Architecture

## Executive Summary

This document describes a production-ready, horizontally scalable email validation system capable of processing millions of validations per day with high accuracy and low latency. The system uses SMTP handshake verification (RCPT TO) without sending actual emails, combined with intelligent caching, rate limiting, and catch-all detection.

**Key Metrics:**
- **Latency**: P95 < 3s, P99 < 5s per validation
- **Throughput**: 10,000+ validations/second (with proper scaling)
- **Accuracy**: >98% for deliverability determination
- **Cost**: ~$0.0001 per validation (at scale)

---

## System Architecture

### Component Diagram

```
┌─────────────────────────────────────────────────────────────────────────┐
│                           Client Applications                            │
│                     (Web UI, API Clients, Batch Jobs)                   │
└────────────────────────────────┬────────────────────────────────────────┘
                                 │
                                 │ HTTPS
                                 ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                           API Gateway + LB                               │
│              (Authentication, Rate Limiting, SSL Termination)           │
└────────────────────────────────┬────────────────────────────────────────┘
                                 │
                                 ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                          API Service (Go)                                │
│  ┌─────────────┐  ┌──────────────┐  ┌────────────────┐                │
│  │   Request   │  │   Response   │  │  Job Manager   │                │
│  │  Validator  │  │  Aggregator  │  │  (Batch Jobs)  │                │
│  └──────┬──────┘  └──────▲───────┘  └────────┬───────┘                │
└─────────┼────────────────┼───────────────────┼─────────────────────────┘
          │                │                   │
          │                │                   │
          ▼                │                   ▼
┌──────────────────┐       │        ┌─────────────────────┐
│  Redis Cache     │       │        │   Message Queue     │
│  ┌────────────┐  │       │        │   (Redis Streams)   │
│  │ MX Records │  │       │        │  ┌──────────────┐   │
│  │  Results   │  │       │        │  │ Validation Q │   │
│  │Rate Limits │  │       │        │  │  Priority Q  │   │
│  └────────────┘  │       │        │  │    DLQ       │   │
└──────────────────┘       │        └──────┬──────────┘   │
                           │               │              │
                           │               ▼              │
          ┌────────────────┼──────────────────────────────┘
          │                │               │
          │                │               │
          ▼                │               ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                     Orchestrator Service (Go)                            │
│  ┌──────────────┐  ┌─────────────────┐  ┌──────────────────┐          │
│  │   Job Queue  │  │   Domain-level  │  │   MX Host Pool   │          │
│  │   Consumer   │  │   Concurrency   │  │   Manager        │          │
│  │              │  │   Controller    │  │                  │          │
│  └──────┬───────┘  └────────┬────────┘  └────────┬─────────┘          │
└─────────┼──────────────────┼───────────────────┼─────────────────────────┘
          │                  │                   │
          │                  │                   │
          ▼                  ▼                   ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                   SMTP Verification Workers (Go Pool)                    │
│  ┌──────────────────────────────────────────────────────────┐          │
│  │                     Worker Instance                       │          │
│  │  ┌────────────┐  ┌─────────────┐  ┌─────────────────┐   │          │
│  │  │ DNS/MX     │→ │ SMTP Conn   │→ │  Catch-all      │   │   x N    │
│  │  │ Resolver   │  │ Pool        │  │  Detector       │   │          │
│  │  └────────────┘  └─────────────┘  └─────────────────┘   │          │
│  │  ┌────────────────────────────────────────────────────┐  │          │
│  │  │         Result Classifier & Publisher             │  │          │
│  │  └────────────────────────────────────────────────────┘  │          │
│  └──────────────────────────────────────────────────────────┘          │
└──────────────────────┬──────────────────────────────────────────────────┘
                       │
                       │ Results
                       ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                      Result Store Service (Go)                           │
│              (Write-optimized, batch inserts)                           │
└────────────────────────────────┬────────────────────────────────────────┘
                                 │
                                 ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                     PostgreSQL Database                                  │
│  ┌──────────────────┐  ┌─────────────────┐  ┌──────────────────┐      │
│  │ validation_      │  │ validation_     │  │ domain_metadata  │      │
│  │ results          │  │ jobs            │  │                  │      │
│  │ (partitioned)    │  │                 │  │                  │      │
│  └──────────────────┘  └─────────────────┘  └──────────────────┘      │
└─────────────────────────────────────────────────────────────────────────┘

External Dependencies:
┌────────────────┐  ┌──────────────┐  ┌────────────────┐
│ DNS Resolvers  │  │  SMTP MX     │  │  Observability │
│ (8.8.8.8, etc) │  │  Servers     │  │  (Prometheus,  │
│                │  │  (Port 25)   │  │   Grafana)     │
└────────────────┘  └──────────────┘  └────────────────┘
```

---

## Data Flow

### Single Email Validation Flow

```
1. Client Request
   POST /v1/validate
   { "email": "user@example.com" }
   │
   ▼
2. API Gateway
   - Authenticate request
   - Rate limit check (per customer)
   - Request validation
   │
   ▼
3. API Service
   - Check Redis cache for recent result
   - If cached → return immediately (200-500ms)
   - If not cached → continue
   │
   ▼
4. Validation Pipeline
   a) Syntax Check
      - RFC 5322 compliance
      - Local part + domain validation
      - Reject obvious invalids
   │
   ▼
   b) DNS/MX Lookup
      - Check Redis cache for MX records
      - If not cached → DNS query (+ cache with TTL)
      - No MX records → return invalid
   │
   ▼
   c) Domain Reputation Check
      - Query domain_metadata table
      - Check for known disposable domains
      - Check for previously detected catch-all
   │
   ▼
5. SMTP Verification (Critical Path)
   a) MX Host Selection
      - Select MX by priority
      - Check current connections to MX host
      - Apply rate limiting (max N connections/second)
   │
   ▼
   b) SMTP Handshake
      - TCP connect to MX:25 (timeout: 10s)
      - EHLO our.hostname.com
      - MAIL FROM:<verify@our.hostname.com>
      - RCPT TO:<user@example.com>
      - Capture response code (250, 550, 450, etc)
      - QUIT
   │
   ▼
   c) Response Classification
      250 → valid
      550/551/553 → invalid
      450/451/452 → temporary failure (greylisting)
      421 → rate limited (backoff)
   │
   ▼
   d) Catch-all Detection (if needed)
      - Test with random email @same.domain
      - If also returns 250 → domain is catch-all
      - Cache result for domain
   │
   ▼
6. Result Storage
   - Write to PostgreSQL (async)
   - Update Redis cache (TTL: 7 days)
   - Update domain metadata
   │
   ▼
7. Response to Client
   {
     "email": "user@example.com",
     "status": "valid|invalid|catch-all|unknown|risky",
     "reason": "mailbox_exists",
     "confidence": 0.98,
     "mx_records": ["mx1.example.com", "mx2.example.com"],
     "smtp_code": 250,
     "is_catch_all": false,
     "checked_at": "2025-11-20T16:00:00Z"
   }
```

### Batch Validation Flow

```
1. Batch Request
   POST /v1/validate/batch
   {
     "emails": ["email1@domain.com", ..., "email1000@domain.com"],
     "webhook_url": "https://client.com/webhook",
     "priority": "standard"
   }
   │
   ▼
2. Job Creation
   - Create validation_job record
   - Split into individual validation tasks
   - Enqueue to Redis Streams with priority
   - Return job_id immediately
   │
   ▼
3. Background Processing
   - Orchestrator consumes from queue
   - Groups by domain for efficiency
   - Distributes to worker pool
   - Respects domain-level rate limits
   │
   ▼
4. Worker Pool Processing
   - Each worker processes emails
   - Results streamed to database
   - Progress updated in real-time
   │
   ▼
5. Completion
   - Update job status to "completed"
   - Trigger webhook notification
   - Results available via GET /v1/jobs/{id}
```

---

## Component Specifications

### 1. API Gateway

**Technology**: nginx or Kong  
**Responsibilities**:
- SSL/TLS termination
- Request authentication (API keys, JWT)
- Rate limiting (per customer tier)
- Request/response logging
- Health check endpoints

**Configuration**:
```yaml
rate_limits:
  free_tier: 100/hour
  standard: 10000/hour
  enterprise: unlimited
timeouts:
  connect: 5s
  read: 30s
  write: 30s
```

### 2. API Service

**Technology**: Go (net/http)  
**Responsibilities**:
- REST API endpoints
- Request validation and sanitization
- Cache management (read/write)
- Job orchestration
- Response formatting

**Endpoints**:
- `POST /v1/validate` - Single validation
- `POST /v1/validate/batch` - Batch validation
- `GET /v1/results/{email}` - Retrieve cached result
- `GET /v1/jobs/{id}` - Job status and results
- `GET /health` - Health check
- `GET /metrics` - Prometheus metrics

**Scaling**: Stateless, horizontal scaling via replica count

### 3. Orchestrator Service

**Technology**: Go  
**Responsibilities**:
- Message queue consumption
- Domain-level concurrency control
- Worker pool coordination
- Retry logic and dead letter queue handling
- Backpressure management

**Concurrency Limits**:
- Max 5 concurrent connections per domain
- Max 50 concurrent connections per MX host
- Global worker pool size: configurable (default 1000)

**Retry Policy**:
- Temporary failures: 3 retries with exponential backoff
- Rate limit (421): backoff based on Retry-After header
- Permanent failures: no retry, mark as failed

### 4. SMTP Verification Workers

**Technology**: Go (net/smtp, custom SMTP client)  
**Responsibilities**:
- DNS/MX resolution
- SMTP connection management
- Handshake execution
- Response parsing and classification
- Catch-all detection
- Result publishing

**Connection Pooling**:
- Maintain connection pools per MX host
- Reuse connections when possible (not all servers support)
- Graceful connection closure
- Connection timeout: 10s
- Read/Write timeout: 15s

**Error Handling**:
- Network errors → retry with backoff
- Timeout → mark as unknown
- Connection refused → mark as invalid (no mail service)
- Greylisting detected → retry after delay

### 5. Result Store Service

**Technology**: Go with pgx driver  
**Responsibilities**:
- Batch insert optimization
- Query API for historical lookups
- Data retention management
- Export functionality (CSV, JSON)

**Performance**:
- Batch inserts (1000 records/batch)
- Async writes with buffering
- Connection pooling (50 connections)

### 6. PostgreSQL Database

**Configuration**:
- Version: 15+
- Partitioning: Range partition by date (monthly)
- Indexes: email hash, domain, validation timestamp
- Replication: Primary + 1 read replica

**Performance Tuning**:
```
shared_buffers = 4GB
effective_cache_size = 12GB
work_mem = 64MB
maintenance_work_mem = 1GB
max_connections = 200
```

### 7. Redis Cache

**Configuration**:
- Version: 7+
- Persistence: RDB snapshots + AOF
- Max memory: 8GB
- Eviction policy: allkeys-lru

**Usage**:
- MX record cache (TTL: 1 hour - 24 hours based on DNS TTL)
- Validation result cache (TTL: 7 days)
- Domain metadata cache (TTL: 24 hours)
- Rate limiting counters (TTL: 1 hour)

### 8. Message Queue

**Technology**: Redis Streams  
**Queues**:
- `validation:express` - Priority: high, SLA: < 5s
- `validation:standard` - Priority: normal, SLA: < 30s
- `validation:bulk` - Priority: low, SLA: < 5m
- `validation:dlq` - Dead letter queue for failed jobs

**Consumer Groups**: Multiple consumers for parallel processing

---

## Result Classification Engine

### Decision Tree

```
Input: SMTP Response Code + Context
│
├─ Syntax Invalid → status: invalid, reason: syntax_error
│
├─ No MX Records → status: invalid, reason: no_mx_records
│
├─ SMTP 250 (Mailbox exists)
│  ├─ Catch-all detected → status: catch-all, confidence: 0.5
│  └─ Not catch-all → status: valid, confidence: 0.98
│
├─ SMTP 550/551/553 (Mailbox not found)
│  └─ status: invalid, reason: mailbox_not_found, confidence: 0.95
│
├─ SMTP 450/451/452 (Temporary failure)
│  └─ status: unknown, reason: temporary_error, confidence: 0.3
│     (recommend retry later)
│
├─ SMTP 421 (Rate limited)
│  └─ status: unknown, reason: rate_limited
│     (retry with backoff)
│
├─ Connection timeout/refused
│  └─ status: unknown, reason: connection_failed, confidence: 0.2
│
├─ Disposable domain detected
│  └─ status: risky, reason: disposable_domain, confidence: 0.9
│
└─ Catch-all domain + suspicious pattern
   └─ status: risky, reason: catch_all_suspicious, confidence: 0.4
```

### Status Definitions

| Status | Meaning | Recommended Action |
|--------|---------|-------------------|
| `valid` | Mailbox exists and can receive mail | Accept email |
| `invalid` | Mailbox does not exist or domain has no MX | Reject email |
| `catch-all` | Domain accepts all emails (mailbox may not exist) | Accept with caution or verify via other means |
| `unknown` | Could not determine (temp failure, timeout) | Accept but flag for review or retry validation later |
| `risky` | Disposable, suspicious, or high-risk domain | Reject or require additional verification |

---

## Performance Targets

### Latency (Single Validation)

| Percentile | Target | Components |
|------------|--------|------------|
| P50 | < 1.5s | Cache hit: ~100ms, Cache miss: DNS (200ms) + SMTP (1s) |
| P95 | < 3s | Includes slower SMTP servers, retries |
| P99 | < 5s | Includes timeouts, backoff |

### Throughput

| Scenario | Target | Scaling Strategy |
|----------|--------|------------------|
| Cached validations | 50,000/sec | Redis can handle millions of reads/sec |
| Uncached validations | 10,000/sec | 1000 workers × 10 validations/sec/worker |
| Batch processing | 1M/hour | Horizontal worker scaling |

### Resource Utilization

| Component | CPU | Memory | Network |
|-----------|-----|--------|---------|
| API Service (1 instance) | 1 core | 512MB | 100 Mbps |
| Orchestrator (1 instance) | 2 cores | 1GB | 100 Mbps |
| Worker (1 instance) | 1 core | 256MB | 50 Mbps |
| PostgreSQL | 4 cores | 16GB | 1 Gbps |
| Redis | 2 cores | 8GB | 1 Gbps |

---

## Scaling Strategy

### Horizontal Scaling

**Auto-scaling Triggers**:
- **API Service**: Scale when CPU > 70% or request queue > 100
- **Workers**: Scale when queue depth > 5000 or latency P95 > 5s
- **Orchestrator**: Scale when message lag > 10s

**Kubernetes HPA Configuration**:
```yaml
minReplicas: 3
maxReplicas: 50
metrics:
  - type: Resource
    resource:
      name: cpu
      targetAverageUtilization: 70
  - type: Pods
    pods:
      metricName: queue_depth
      targetAverageValue: 500
```

### Vertical Scaling

- **PostgreSQL**: Increase to 8 cores, 32GB RAM for > 10M validations/day
- **Redis**: Increase to 4 cores, 16GB RAM for larger cache needs

### Geographic Distribution

For global deployments:
- Deploy in multiple regions (US-East, US-West, EU, Asia)
- Route requests to nearest region
- Replicate PostgreSQL cross-region
- Use Redis Cluster for distributed caching

---

## Cost Efficiency

### Optimization Strategies

1. **Aggressive Caching**:
   - Cache validation results for 7 days
   - Cache MX records with DNS TTL
   - Estimated cache hit rate: 60-80%
   - Cost savings: 60-80% reduction in SMTP connections

2. **Connection Reuse**:
   - Maintain SMTP connection pools
   - Reuse connections for same MX host
   - Reduces connection overhead by 50%

3. **Batch Processing**:
   - Group validations by domain
   - Share MX lookups across emails
   - Reduces DNS queries by 90% for batch jobs

4. **Smart Retry Logic**:
   - Exponential backoff for temporary failures
   - Respect rate limit signals
   - Avoid unnecessary retries

### Cost Breakdown (estimated)

For 10M validations/day:

| Component | Monthly Cost | Notes |
|-----------|-------------|-------|
| Kubernetes Cluster | $300 | 10 nodes × $10/day |
| PostgreSQL (managed) | $200 | 4 cores, 16GB, 500GB SSD |
| Redis (managed) | $100 | 2 cores, 8GB |
| Network egress | $50 | ~500GB SMTP traffic |
| **Total** | **$650** | **~$0.002 per validation** |

With 80% cache hit rate:
- Actual SMTP validations: 2M/day
- **Effective cost: ~$0.0001 per validation**

---

## Security Considerations

### Network Security

1. **Egress IPs**:
   - Use dedicated egress IPs for SMTP connections
   - Set up proper PTR records (reverse DNS)
   - Configure SPF records for MAIL FROM domain
   - Rotate IPs if blacklisted

2. **SMTP Security**:
   - Use legitimate MAIL FROM addresses
   - Implement EHLO with valid hostname
   - Respect rate limits and backoff signals
   - Never send actual email content

3. **Firewall Rules**:
   - Outbound: Allow port 25 (SMTP), 53 (DNS)
   - Inbound: API gateway only (443)
   - Database: Internal network only

### Application Security

1. **API Security**:
   - API key authentication
   - Rate limiting per customer
   - Input validation and sanitization
   - SQL injection prevention (parameterized queries)

2. **Data Privacy**:
   - Hash email addresses in logs
   - Encrypt sensitive data at rest
   - GDPR compliance (data retention, deletion)
   - Audit logging for all API requests

3. **DDoS Protection**:
   - Rate limiting at API gateway
   - Request size limits
   - Connection limits
   - Geographic blocking if needed

### Abuse Prevention

1. **Rate Limiting**:
   - Per customer tier limits
   - Per domain limits (max 100/minute)
   - Per MX host limits (max 50 concurrent)
   - Global system limits

2. **Reputation Management**:
   - Monitor for blacklisting of egress IPs
   - Implement IP rotation
   - Respect SMTP server signals (421, Retry-After)
   - Maintain good sender reputation

---

## Anti-Patterns to Avoid

1. **Sending Actual Emails**: Never send DATA command or actual email content
2. **Ignoring Rate Limits**: Always respect 421 responses and backoff
3. **No Connection Limits**: Always limit concurrent connections per MX
4. **Infinite Retries**: Implement max retry counts and circuit breakers
5. **No Caching**: Always cache MX records and recent validation results
6. **Synchronous Batch Processing**: Always use async queue for batch jobs
7. **Single Egress IP**: Use multiple IPs to avoid blacklisting

---

## Observability

See [metrics.md](file:///Users/bigwolf/.gemini/antigravity/scratch/mail_sorter/docs/metrics.md) for detailed metrics specification.

### Key Metrics

- **Latency**: validation_duration_seconds (histogram)
- **Throughput**: validations_total (counter)
- **Error Rate**: validation_errors_total (counter by type)
- **Cache Hit Rate**: cache_hits_total / cache_requests_total
- **Queue Depth**: queue_depth_current (gauge)
- **SMTP Codes**: smtp_response_codes_total (counter by code)

### Alerting

- P95 latency > 5s for 5 minutes
- Error rate > 5% for 5 minutes
- Queue depth > 10,000 for 5 minutes
- Cache hit rate < 50% for 15 minutes
- Worker pool utilization > 90% for 5 minutes

---

## Next Steps

1. Review [schema.sql](file:///Users/bigwolf/.gemini/antigravity/scratch/mail_sorter/database/schema.sql) for database design
2. Review [smtp-verifier.go](file:///Users/bigwolf/.gemini/antigravity/scratch/mail_sorter/services/verifier/smtp-verifier.go) for implementation
3. Review [kubernetes.yaml](file:///Users/bigwolf/.gemini/antigravity/scratch/mail_sorter/deploy/kubernetes.yaml) for deployment
4. Review [runbook.md](file:///Users/bigwolf/.gemini/antigravity/scratch/mail_sorter/docs/runbook.md) for operations
