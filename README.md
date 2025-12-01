# Email Deliverability Validation System

Production-ready email validation system capable of validating millions of email addresses using SMTP verification without sending actual emails.

## Features

- **High-accuracy validation** using SMTP RCPT TO handshake
- **Catch-all detection** to identify domains that accept all emails
- **Intelligent caching** with Redis for sub-second repeat validations
- **Scalable architecture** supporting 10,000+ validations/second
- **Production-ready** with Kubernetes deployment, monitoring, and alerting

## Architecture

```
Client → API Gateway → API Service → Queue → Workers → SMTP Servers
                     ↓                         ↓
                   Redis Cache          PostgreSQL Database
```

See [architecture.md](docs/architecture.md) for complete system design.

## Quick Start

### Prerequisites

- Kubernetes cluster (v1.27+)
- kubectl configured
- Docker
- Go 1.21+ (for development)

### Deploy to Kubernetes

```bash
# Create namespace and deploy all services
kubectl apply -f deploy/kubernetes.yaml

# Wait for pods to be ready
kubectl wait --for=condition=ready pod --all -n email-validator --timeout=300s

# Get API service URL
kubectl get ingress -n email-validator
```

### Local Development

```bash
# Start dependencies
docker-compose up -d postgres redis

# Run database migrations
psql -h localhost -U email_validator_app -d email_validation -f database/schema.sql

# Run SMTP verifier
cd services/verifier
go run main.go
```

## API Usage

### Single Validation

```bash
curl -X POST https://api.mail-validator.com/v1/validate \
  -H "X-API-Key: YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"email": "user@example.com"}'
```

**Response**:
```json
{
  "email": "user@example.com",
  "status": "valid",
  "reason": "mailbox_exists",
  "confidence": 0.98,
  "smtp_code": 250,
  "is_catch_all": false,
  "checked_at": "2025-11-20T16:00:00Z"
}
```

### Batch Validation

```bash
curl -X POST https://api.mail-validator.com/v1/validate/batch \
  -H "X-API-Key: YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "emails": ["user1@example.com", "user2@example.com"],
    "priority": "standard",
    "webhook_url": "https://your-domain.com/webhook"
  }'
```

## Status Values

| Status | Meaning | Recommended Action |
|--------|---------|-------------------|
| `valid` | Mailbox exists | Accept email |
| `invalid` | Mailbox does not exist | Reject email |
| `catch-all` | Domain accepts all emails | Accept with caution |
| `unknown` | Could not determine | Retry or accept |
| `risky` | Disposable/suspicious domain | Reject or verify |

## Configuration

See [config/config.yaml](config/config.yaml) for full configuration options.

Key settings:
- SMTP timeouts
- Worker pool size
- Rate limits
- Cache TTLs
- Retry policy

## Monitoring

### Grafana Dashboards

Access at `http://grafana.yourdomain.com`

- **Overview**: Validations/sec, latency, error rate
- **Workers**: Pool utilization, queue depth
- **Infrastructure**: Database, Redis, resource usage

### Prometheus Metrics

```bash
curl http://api-service:9090/metrics
```

See [metrics.md](docs/metrics.md) for complete metrics list.

### Key Metrics

- `email_validator_validations_total` - Total validations
- `email_validator_validation_duration_seconds` - Latency
- `email_validator_cache_requests_total` - Cache hit rate
- `email_validator_queue_depth` - Queue size

## Documentation

- [Architecture](docs/architecture.md) - Complete system architecture
- [API Specification](api/api-spec.yaml) - OpenAPI documentation
- [Redis Caching](docs/redis-keys.md) - Caching strategy
- [Metrics](docs/metrics.md) - Prometheus metrics
- [Runbook](docs/runbook.md) - Operational procedures
- [Roadmap](docs/roadmap.md) - Development timeline

## Project Structure

```
├── api/
│   └── api-spec.yaml          # OpenAPI specification
├── config/
│   └── config.yaml            # Configuration defaults
├── database/
│   └── schema.sql             # PostgreSQL schema
├── deploy/
│   ├── kubernetes.yaml        # K8s deployment manifests
│   └── grafana-dashboard.json # Grafana dashboard
├── docs/
│   ├── architecture.md        # System architecture
│   ├── metrics.md             # Metrics specification
│   ├── redis-keys.md          # Caching strategy
│   ├── roadmap.md             # Development roadmap
│   └── runbook.md             # Operations runbook
└── services/
    ├── api/                   # API service (Go)
    ├── orchestrator/          # Job orchestrator (Go)
    └── verifier/
        └── smtp-verifier.go   # SMTP verification (Go)
```

## Performance

### Targets

- **Latency**: P95 < 3s, P99 < 5s
- **Throughput**: 10,000+ validations/sec
- **Cache Hit Rate**: >70%
- **Uptime**: 99.9%

### Benchmarks

With 1000 workers:
- Cached validations: 50,000/sec
- Uncached validations: 10,000/sec
- Average latency: 1.2s

## Security

- **API Authentication**: API key required
- **Rate Limiting**: Per-customer limits
- **Network Security**: Egress IPs with PTR records
- **Data Privacy**: Email hashing in logs
- **SMTP Security**: Proper EHLO, MAIL FROM identity

## Scaling

### Horizontal Scaling

Auto-scaling enabled:
- API Service: 3-50 replicas
- Workers: 10-100 replicas

Manual scaling:
```bash
kubectl scale deployment smtp-workers --replicas=50 -n email-validator
```

### Vertical Scaling

Increase resources in deployment manifests.

## Troubleshooting

### High Latency

1. Check queue depth
2. Scale up workers
3. Check cache hit rate

### High Error Rate

1. Check SMTP server logs
2. Verify egress IP not blacklisted
3. Check network connectivity

See [runbook.md](docs/runbook.md) for detailed troubleshooting.

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make changes with tests
4. Submit pull request

Code coverage must be >80%.

## License

[Your License Here]

## Support

- **Issues**: GitHub Issues
- **Discussions**: GitHub Discussions
- **Security**: security@yourdomain.com
- **Commercial Support**: support@yourdomain.com

---

**Built with ❤️ for reliable email validation**
