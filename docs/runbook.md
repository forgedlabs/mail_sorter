# Operational Runbook

## Overview

This runbook provides procedures for operating the email validation system in production.

**Audience**: DevOps engineers, SREs, on-call engineers  
**Last Updated**: 2025-11-20

---

## Table of Contents

1. [Service Overview](#service-overview)
2. [Architecture Quick Reference](#architecture-quick-reference)
3. [Starting and Stopping Services](#starting-and-stopping-services)
4. [Health Checks](#health-checks)
5. [Common Operations](#common-operations)
6. [Troubleshooting](#troubleshooting)
7. [Incident Response](#incident-response)
8. [Maintenance Procedures](#maintenance-procedures)
9. [Scaling](#scaling)
10. [Monitoring and Alerts](#monitoring-and-alerts)

---

## Service Overview

### Components

| Service | Purpose | Critical | Port |
|---------|---------|----------|------|
| API Service | REST API endpoints | Yes | 8080 |
| Orchestrator | Job scheduling | Yes | N/A |
| SMTP Workers | Email verification | Yes | N/A |
| PostgreSQL | Data storage | Yes | 5432 |
| Redis | Caching & queuing | Yes | 6379 |

### Service Dependencies

```
API Service → Redis (cache)
           → PostgreSQL (results lookup)

Orchestrator → Redis (queue)
            → PostgreSQL (job tracking)

Workers → Redis (cache, queue)
        → PostgreSQL (result storage)
        → External SMTP servers
```

---

## Architecture Quick Reference

```
Client → API Gateway → API Service → Queue → Orchestrator → Workers → SMTP
                     ↓                                      ↓
                   Cache (Redis)                     Database (PostgreSQL)
```

**Data Flow**:
1. Client sends validation request to API
2. API checks cache, returns if found
3. If not cached, enqueues to Redis Streams
4. Orchestrator distributes to workers
5. Workers perform SMTP verification
6. Results stored in PostgreSQL and cached in Redis
7. API returns response to client

---

## Starting and Stopping Services

### Start All Services

```bash
# Using Kubernetes
kubectl apply -f deploy/kubernetes.yaml

# Wait for all pods to be ready
kubectl wait --for=condition=ready pod -l app=api-service -n email-validator --timeout=300s
kubectl wait --for=condition=ready pod -l app=smtp-workers -n email-validator --timeout=300s
```

### Stop All Services

```bash
# Scale down to zero (preserves data)
kubectl scale deployment --all --replicas=0 -n email-validator

# Or delete entire namespace (removes everything)
kubectl delete namespace email-validator
```

### Restart Specific Service

```bash
# Restart API service
kubectl rollout restart deployment/api-service -n email-validator

# Restart workers
kubectl rollout restart deployment/smtp-workers -n email-validator
```

### Check Service Status

```bash
# Check all pods
kubectl get pods -n email-validator

# Check specific service
kubectl get deployment api-service -n email-validator

# View logs
kubectl logs -f deployment/api-service -n email-validator
```

---

## Health Checks

### API Health Check

```bash
curl https://api.mail-validator.com/health

# Expected response:
{
  "status": "healthy",
  "version": "1.0.0",
  "timestamp": "2025-11-20T16:00:00Z",
  "checks": {
    "database": true,
    "redis": true,
    "queue": true
  }
}
```

### Database Health

```bash
kubectl exec -it postgresql-0 -n email-validator -- psql -U email_validator_app -d email_validation -c "SELECT 1;"
```

### Redis Health

```bash
kubectl exec -it redis-0 -n email-validator -- redis-cli ping
# Expected: PONG
```

### Check Worker Status

```bash
# Check queue depth
kubectl exec -it redis-0 -n email-validator -- redis-cli XLEN queue:validation:standard

# Check active workers
kubectl get pods -l app=smtp-workers -n email-validator --field-selector=status.phase=Running
```

---

## Common Operations

### Get Validation Result

```bash
# Single validation
curl -X POST https://api.mail-validator.com/v1/validate \
  -H "X-API-Key: YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"email": "test@example.com"}'
```

### Check Job Status

```bash
curl https://api.mail-validator.com/v1/jobs/{JOB_ID} \
  -H "X-API-Key: YOUR_API_KEY"
```

### View Queue Depth

```bash
kubectl exec -it redis-0 -n email-validator -- redis-cli XLEN queue:validation:standard
```

### Clear Cache (Use with caution)

```bash
# Clear specific email cache
kubectl exec -it redis-0 -n email-validator -- redis-cli DEL validation:result:HASH

# Clear all validation results (DANGEROUS)
kubectl exec -it redis-0 -n email-validator -- redis-cli --scan --pattern "validation:result:*" | xargs kubectl exec -it redis-0 -n email-validator -- redis-cli DEL
```

### Drain Queue

```bash
# Read all messages from queue
kubectl exec -it redis-0 -n email-validator -- redis-cli XREAD COUNT 100 STREAMS queue:validation:standard 0
```

---

## Troubleshooting

### High Latency

**Symptoms**: P95 latency > 5s

**Diagnosis**:
```bash
# Check worker utilization
kubectl top pods -l app=smtp-workers -n email-validator

# Check queue depth
kubectl exec -it redis-0 -n email-validator -- redis-cli XLEN queue:validation:standard

# Check database connections
kubectl exec -it postgresql-0 -n email-validator -- psql -U email_validator_app -d email_validation -c "SELECT count(*) FROM pg_stat_activity;"
```

**Solutions**:
1. Scale up workers: `kubectl scale deployment smtp-workers --replicas=20 -n email-validator`
2. Check for slow SMTP servers in logs
3. Increase Redis memory if cache hit rate is low
4. Check database query performance

### High Error Rate

**Symptoms**: Error rate > 5%

**Diagnosis**:
```bash
# Check errors in logs
kubectl logs deployment/api-service -n email-validator | grep ERROR

# Check SMTP errors
kubectl logs deployment/smtp-workers -n email-validator | grep "SMTP Error"

# Check metrics
curl http://api-service:9090/metrics | grep email_validator_errors_total
```

**Common Causes**:
- **550 errors**: Invalid emails (expected)
- **Timeouts**: SMTP server issues or network problems
- **Connection refused**: MX host down
- **421 Rate limited**: Need to back off

**Solutions**:
1. Check if specific MX hosts are problematic
2. Verify egress IP is not blacklisted
3. Reduce rate limiting if getting 421 errors
4. Check network connectivity to external SMTP servers

### Queue Backup

**Symptoms**: Queue depth > 10,000

**Diagnosis**:
```bash
# Check queue depth
kubectl exec -it redis-0 -n email-validator -- redis-cli XLEN queue:validation:standard

# Check worker count
kubectl get pods -l app=smtp-workers -n email-validator | grep Running | wc -l

# Check worker logs for errors
kubectl logs deployment/smtp-workers -n email-validator --tail=100
```

**Solutions**:
1. Scale up workers: `kubectl scale deployment smtp-workers --replicas=50 -n email-validator`
2. Check if workers are stuck (restart if necessary)
3. Verify workers can connect to Redis and PostgreSQL
4. Consider moving to higher priority queue

### Cache Hit Rate Low

**Symptoms**: Cache hit rate < 50%

**Diagnosis**:
```bash
# Check Redis memory
kubectl exec -it redis-0 -n email-validator -- redis-cli INFO MEMORY

# Check eviction stats
kubectl exec -it redis-0 -n email-validator -- redis-cli INFO STATS | grep evicted_keys
```

**Solutions**:
1. Increase Redis memory allocation
2. Adjust TTL values
3. Check if cache is being cleared unnecessarily
4. Verify eviction policy is set correctly

### Database Connection Pool Exhausted

**Symptoms**: "too many connections" errors

**Diagnosis**:
```bash
kubectl exec -it postgresql-0 -n email-validator -- psql -U email_validator_app -d email_validation -c "SELECT count(*), state FROM pg_stat_activity GROUP BY state;"
```

**Solutions**:
1. Increase max_connections in PostgreSQL config
2. Reduce connection pool size in application
3. Check for connection leaks (connections not being closed)
4. Add read replica for queries

---

## Incident Response

### Severity Levels

| Level | Description | Response Time | Example |
|-------|-------------|---------------|---------|
| P0 | Service down | Immediate | All APIs returning 503 |
| P1 | Critical degradation | 15 minutes | Error rate > 25% |
| P2 | Partial degradation | 1 hour | Latency > 10s |
| P3 | Minor issues | Next day | Cache hit rate low |

### P0: Service Down

**Actions**:
1. Check Kubernetes pods: `kubectl get pods -n email-validator`
2. Check recent deployments: `kubectl rollout history deployment/api-service -n email-validator`
3. Check logs: `kubectl logs deployment/api-service -n email-validator --tail=100`
4. Rollback if recent deployment: `kubectl rollout undo deployment/api-service -n email-validator`
5. Check dependencies (PostgreSQL, Redis)
6. Notify team via Slack/PagerDuty

### P1: High Error Rate

**Actions**:
1. Identify error source (API, workers, database)
2. Check metrics dashboard
3. Review recent changes
4. Implement mitigation (scale up, rollback, circuit breaker)
5. Create incident ticket
6. Post-incident review within 24 hours

### Communication Template

```
🚨 INCIDENT ALERT
Severity: [P0/P1/P2]
Component: [API/Workers/Database]
Impact: [Description]
Status: [Investigating/Mitigating/Resolved]
ETA: [Time]
Updates: Every 15 minutes

Actions Taken:
- [Action 1]
- [Action 2]

Next Steps:
- [Step 1]
- [Step 2]
```

---

## Maintenance Procedures

### Database Backup

```bash
# Manual backup
kubectl exec -it postgresql-0 -n email-validator -- pg_dump -U email_validator_app email_validation > backup_$(date +%Y%m%d).sql

# Restore from backup
kubectl exec -i postgresql-0 -n email-validator -- psql -U email_validator_app email_validation < backup_20251120.sql
```

### Database Partition Maintenance

```sql
-- Create next month's partition (run monthly)
SELECT create_next_partition();

-- Drop old partitions (run quarterly)
DROP TABLE IF EXISTS validation_results_2024_01;
```

### Clear Old Data

```sql
-- Delete validation results older than 90 days
SELECT cleanup_old_validations(90);
```

### Redis Maintenance

```bash
# Check memory usage
kubectl exec -it redis-0 -n email-validator -- redis-cli INFO MEMORY

# Trigger manual save
kubectl exec -it redis-0 -n email-validator -- redis-cli BGSAVE

# Check persistence
kubectl exec -it redis-0 -n email-validator -- redis-cli LASTSAVE
```

### Certificate Renewal

```bash
# Check certificate expiration
kubectl get certificate -n email-validator

# Force renewal
kubectl delete certificate api-tls-cert -n email-validator
# cert-manager will automatically recreate
```

### Log Rotation

Logs are automatically rotated by Kubernetes. Retention: 30 days.

```bash
# View recent logs
kubectl logs deployment/api-service -n email-validator --since=1h

# View logs from specific time
kubectl logs deployment/api-service -n email-validator --since-time=2025-11-20T10:00:00Z
```

---

## Scaling

### Manual Scaling

```bash
# Scale API service
kubectl scale deployment api-service --replicas=10 -n email-validator

# Scale workers
kubectl scale deployment smtp-workers --replicas=50 -n email-validator

# Scale orchestrator
kubectl scale deployment orchestrator --replicas=3 -n email-validator
```

### Autoscaling Configuration

Autoscaling is configured via HorizontalPodAutoscaler:

```bash
# Check HPA status
kubectl get hpa -n email-validator

# Edit HPA
kubectl edit hpa api-service-hpa -n email-validator
```

**Autoscaling Triggers**:
- API Service: CPU > 70%, Active requests > 100
- Workers: CPU > 70%, Queue depth > 500

### Vertical Scaling (Resources)

```bash
# Edit deployment to change resource limits
kubectl edit deployment api-service -n email-validator

# Example resource changes:
resources:
  requests:
    cpu: 1000m      # Changed from 500m
    memory: 1Gi     # Changed from 512Mi
  limits:
    cpu: 2000m
    memory: 2Gi
```

---

## Monitoring and Alerts

### Key Dashboards

1. **Overview Dashboard**: https://grafana.yourdomain.com/d/email-validator-overview
   - Validations/sec
   - P95 latency
   - Error rate
   - Cache hit rate

2. **Worker Dashboard**: https://grafana.yourdomain.com/d/email-validator-workers
   - Worker utilization
   - Queue depth
   - SMTP response codes

3. **Infrastructure Dashboard**: https://grafana.yourdomain.com/d/email-validator-infra
   - Database connections
   - Redis memory
   - Pod resources

### Alert Rules

Configured in Prometheus. See `deploy/kubernetes.yaml` PrometheusRule.

**Critical Alerts** (PagerDuty):
- High error rate (> 5%)
- Service down
- Database connection pool exhausted

**Warning Alerts** (Slack):
- High latency (P95 > 5s)
- Queue depth high (> 10,000)
- Low cache hit rate (< 50%)
- Worker utilization high (> 90%)

### Metrics Endpoints

```bash
# API service metrics
curl http://api-service:9090/metrics

# Worker metrics
curl http://smtp-workers:9090/metrics

# Aggregated metrics (Prometheus)
curl http://prometheus:9090/api/v1/query?query=email_validator_validations_total
```

---

## Contact Information

**On-Call Rotation**: See PagerDuty schedule  
**Slack Channel**: #email-validator-alerts  
**Incident Tracking**: JIRA Project: EMAILVAL  
**Documentation**: Confluence/Notion

**Escalation Path**:
1. On-call engineer
2. Team lead
3. Engineering manager
4. CTO

---

## Related Documents

- [architecture.md](file:///Users/bigwolf/.gemini/antigravity/scratch/mail_sorter/docs/architecture.md) - System architecture
- [metrics.md](file:///Users/bigwolf/.gemini/antigravity/scratch/mail_sorter/docs/metrics.md) - Prometheus metrics
- [kubernetes.yaml](file:///Users/bigwolf/.gemini/antigravity/scratch/mail_sorter/deploy/kubernetes.yaml) - K8s deployment
- [roadmap.md](file:///Users/bigwolf/.gemini/antigravity/scratch/mail_sorter/docs/roadmap.md) - Development roadmap
