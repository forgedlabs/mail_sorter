# Redis Caching Strategy

## Overview

Redis is used for high-performance caching, rate limiting, and temporary data storage. This document defines key naming conventions, TTL policies, and usage patterns.

## Key Naming Convention

All keys follow the pattern: `{namespace}:{resource}:{identifier}:{subkey}`

### Namespaces

- `mx:` - MX record cache
- `validation:` - Validation result cache
- `domain:` - Domain metadata cache
- `ratelimit:` - Rate limiting counters
- `queue:` - Message queue (Redis Streams)
- `lock:` - Distributed locks
- `stats:` - Statistics and metrics

---

## Cache Keys and Patterns

### 1. MX Record Cache

**Key Pattern**: `mx:records:{domain}`

**Value**: JSON array of MX records
```json
[
  {"exchange": "mx1.example.com", "priority": 10},
  {"exchange": "mx2.example.com", "priority": 20}
]
```

**TTL**: Dynamic based on DNS TTL (min: 1 hour, max: 24 hours)

**Usage**:
```redis
SET mx:records:gmail.com '[{"exchange":"gmail-smtp-in.l.google.com","priority":5}]' EX 3600
GET mx:records:gmail.com
```

**Eviction**: TTL-based expiration

---

### 2. Validation Result Cache

**Key Pattern**: `validation:result:{email_hash}`

**Value**: JSON object with validation result
```json
{
  "email": "hashed",
  "status": "valid",
  "reason": "mailbox_exists",
  "confidence": 0.98,
  "smtp_code": 250,
  "is_catch_all": false,
  "mx_host": "mx1.example.com",
  "checked_at": "2025-11-20T16:00:00Z"
}
```

**TTL**: 7 days (604800 seconds)

**Usage**:
```redis
SET validation:result:abc123... '{"status":"valid",...}' EX 604800
GET validation:result:abc123...
```

**Eviction**: TTL expires or LRU if memory limit reached

---

### 3. Domain Metadata Cache

**Key Pattern**: `domain:meta:{domain}`

**Value**: JSON object with domain information
```json
{
  "is_catch_all": false,
  "catch_all_checked_at": "2025-11-20T15:00:00Z",
  "is_disposable": false,
  "mx_records": [...],
  "last_validation": "2025-11-20T16:00:00Z"
}
```

**TTL**: 24 hours (86400 seconds)

**Usage**:
```redis
SET domain:meta:example.com '{"is_catch_all":false,...}' EX 86400
GET domain:meta:example.com
```

**Eviction**: TTL-based

---

### 4. Catch-All Detection Cache

**Key Pattern**: `domain:catchall:{domain}`

**Value**: Boolean (0 or 1) or JSON

**TTL**: 7 days (604800 seconds) - domain behavior unlikely to change

**Usage**:
```redis
SET domain:catchall:example.com 1 EX 604800
GET domain:catchall:example.com
```

---

### 5. Rate Limiting

#### Per Customer Rate Limit

**Key Pattern**: `ratelimit:customer:{customer_id}:{window}`

**Value**: Counter

**TTL**: Window duration (e.g., 3600 for hourly)

**Usage**:
```redis
INCR ratelimit:customer:cust123:hour:2025-11-20-16
EXPIRE ratelimit:customer:cust123:hour:2025-11-20-16 3600
```

#### Per Domain Rate Limit

**Key Pattern**: `ratelimit:domain:{domain}:{timestamp_second}`

**Value**: Counter

**TTL**: 60 seconds

**Usage**:
```redis
# Sliding window: allow max 5 requests per domain per second
INCR ratelimit:domain:example.com:1700494820
EXPIRE ratelimit:domain:example.com:1700494820 60
```

#### Per MX Host Rate Limit

**Key Pattern**: `ratelimit:mx:{mx_host}:{timestamp_second}`

**Value**: Counter (current connections)

**TTL**: 60 seconds

**Usage**:
```redis
# Track concurrent connections to MX host
INCR ratelimit:mx:mx1.example.com:connections
DECR ratelimit:mx:mx1.example.com:connections
```

---

### 6. Message Queue (Redis Streams)

**Stream Names**:
- `queue:validation:express`
- `queue:validation:standard`
- `queue:validation:bulk`
- `queue:validation:dlq` (dead letter queue)

**Consumer Groups**: `validators`

**Usage**:
```redis
XADD queue:validation:standard * email user@example.com job_id 12345 priority standard
XREADGROUP GROUP validators worker1 COUNT 10 STREAMS queue:validation:standard >
```

**Trimming**: MAXLEN ~ 10000 (approximate)

---

### 7. Distributed Locks

**Key Pattern**: `lock:{resource}:{identifier}`

**Value**: Lock holder ID

**TTL**: 30 seconds (lock timeout)

**Usage**:
```redis
# Acquire lock with NX (only if not exists) and EX (expiration)
SET lock:domain:example.com worker-123 NX EX 30

# Release lock
DEL lock:domain:example.com
```

---

### 8. Statistics and Metrics

**Key Patterns**:
- `stats:validations:total:{date}` - Daily validation count
- `stats:validations:by_status:{status}:{date}` - Count by status
- `stats:customer:{customer_id}:{date}` - Per customer stats

**Value**: Counter

**TTL**: 30 days

**Usage**:
```redis
INCR stats:validations:total:2025-11-20
INCR stats:validations:by_status:valid:2025-11-20
HINCRBY stats:customer:cust123:2025-11-20 validations 1
```

---

## TTL Policy Summary

| Key Type | TTL | Rationale |
|----------|-----|-----------|
| MX Records | 1-24 hours | Based on DNS TTL |
| Validation Results | 7 days | Email deliverability can change, but rarely in short term |
| Domain Metadata | 24 hours | Balance freshness vs. performance |
| Catch-All Status | 7 days | Domain configuration stable |
| Rate Limit Counters | 60-3600 sec | Based on rate limit window |
| Distributed Locks | 30 seconds | Prevent deadlocks |
| Queue Messages | No TTL | Processed or moved to DLQ |
| Statistics | 30 days | Historical data retention |

---

## Memory Management

### Configuration

```redis
maxmemory 8gb
maxmemory-policy allkeys-lru
maxmemory-samples 5
```

### Eviction Strategy

- **Policy**: `allkeys-lru` - Evict least recently used keys when memory limit reached
- **Priority**: 
  1. Keep validation results (most valuable)
  2. Keep MX records (frequently accessed)
  3. Evict old statistics first

### Memory Estimation

For 1M cached validations:
- Average key size: ~100 bytes
- Average value size: ~500 bytes
- Total per validation: ~600 bytes
- **1M validations ≈ 600 MB**

With 8GB Redis:
- Can cache ~13M validation results
- Plus MX records, domain metadata, etc.

---

## Cache Warming

### On Application Startup

1. **Pre-populate Disposable Domains**:
```redis
SADD disposable:domains tempmail.com guerrillamail.com 10minutemail.com
```

2. **Load Popular MX Records**:
```bash
# Load MX for top 1000 domains from database
```

### Periodic Refresh

Run every hour via cron:
```bash
# Refresh MX records expiring in next 30 minutes
# Refresh catch-all status for active domains
```

---

## Cache Invalidation

### Manual Invalidation

```redis
# Invalidate specific email validation
DEL validation:result:{email_hash}

# Invalidate all results for a domain
SCAN 0 MATCH validation:result:*@example.com* COUNT 100
# Then DEL each key

# Invalidate domain metadata
DEL domain:meta:example.com
DEL domain:catchall:example.com
DEL mx:records:example.com
```

### Automatic Invalidation

- TTL expires (primary method)
- LRU eviction when memory limit reached
- Application-triggered on domain reputation change

---

## High Availability

### Persistence

**RDB Snapshots**:
```redis
save 900 1      # After 900 sec if at least 1 key changed
save 300 10     # After 300 sec if at least 10 keys changed
save 60 10000   # After 60 sec if at least 10000 keys changed
```

**AOF (Append Only File)**:
```redis
appendonly yes
appendfsync everysec
```

### Replication

- **Primary**: Write operations
- **Replicas**: Read operations (2+ replicas for HA)
- **Sentinel**: Automatic failover

---

## Monitoring

### Key Metrics to Track

```redis
INFO memory
INFO stats
INFO replication
INFO persistence
```

**Important Metrics**:
- `used_memory` - Current memory usage
- `used_memory_peak` - Peak memory usage
- `evicted_keys` - Keys evicted due to maxmemory
- `keyspace_hits` / `keyspace_misses` - Cache hit rate
- `expired_keys` - Keys expired by TTL

**Cache Hit Rate Calculation**:
```
hit_rate = keyspace_hits / (keyspace_hits + keyspace_misses)
```

**Target**: >80% hit rate for validation results

---

## Best Practices

1. **Always Set TTL**: Never create keys without expiration (except queues)
2. **Use Pipelining**: Batch multiple GET/SET operations
3. **Avoid Large Keys**: Keep values < 1MB (target < 10KB)
4. **Monitor Memory**: Alert when `used_memory` > 75% of `maxmemory`
5. **Use Hashes for Related Data**: More memory efficient than multiple keys
6. **Scan Don't KEYS**: Use SCAN for iterating, never KEYS in production

---

## Example Usage in Application

### Go Example

```go
package cache

import (
    "context"
    "encoding/json"
    "time"
    "github.com/redis/go-redis/v9"
)

type ValidationResult struct {
    Status     string    `json:"status"`
    Reason     string    `json:"reason"`
    Confidence float64   `json:"confidence"`
    CheckedAt  time.Time `json:"checked_at"`
}

func GetValidationResult(ctx context.Context, rdb *redis.Client, emailHash string) (*ValidationResult, error) {
    key := "validation:result:" + emailHash
    val, err := rdb.Get(ctx, key).Result()
    if err == redis.Nil {
        return nil, nil // Cache miss
    }
    if err != nil {
        return nil, err
    }
    
    var result ValidationResult
    err = json.Unmarshal([]byte(val), &result)
    return &result, err
}

func SetValidationResult(ctx context.Context, rdb *redis.Client, emailHash string, result *ValidationResult) error {
    key := "validation:result:" + emailHash
    data, err := json.Marshal(result)
    if err != nil {
        return err
    }
    
    ttl := 7 * 24 * time.Hour // 7 days
    return rdb.Set(ctx, key, data, ttl).Err()
}

func GetMXRecords(ctx context.Context, rdb *redis.Client, domain string) ([]MXRecord, error) {
    key := "mx:records:" + domain
    val, err := rdb.Get(ctx, key).Result()
    if err == redis.Nil {
        return nil, nil // Cache miss - query DNS
    }
    if err != nil {
        return nil, err
    }
    
    var records []MXRecord
    err = json.Unmarshal([]byte(val), &records)
    return records, err
}

func CheckRateLimit(ctx context.Context, rdb *redis.Client, customerID string, limit int) (bool, error) {
    now := time.Now()
    window := now.Format("2006-01-02-15") // Hour window
    key := "ratelimit:customer:" + customerID + ":hour:" + window
    
    count, err := rdb.Incr(ctx, key).Result()
    if err != nil {
        return false, err
    }
    
    if count == 1 {
        // First request in this window - set expiration
        rdb.Expire(ctx, key, 1*time.Hour)
    }
    
    return count <= int64(limit), nil
}
```

---

## Related Documents

- [architecture.md](file:///Users/bigwolf/.gemini/antigravity/scratch/mail_sorter/docs/architecture.md) - System architecture overview
- [schema.sql](file:///Users/bigwolf/.gemini/antigravity/scratch/mail_sorter/database/schema.sql) - PostgreSQL database schema
- [smtp-verifier.go](file:///Users/bigwolf/.gemini/antigravity/scratch/mail_sorter/services/verifier/smtp-verifier.go) - SMTP verification implementation
