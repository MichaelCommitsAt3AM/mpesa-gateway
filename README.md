# M-Pesa Payment Gateway Microservice

A production-grade, security-hardened payment gateway for integrating Safaricom's M-Pesa STK Push. Built with Go following "boring technology" principles.

## Architecture Overview

```
┌─────────────┐      ┌──────────────┐      ┌─────────────┐
│   Client    │─────▶│  API Server  │─────▶│  Safaricom  │
│  (Tenant)   │      │   (Chi/HTTP) │      │   M-Pesa    │
└─────────────┘      └──────┬───────┘      └─────────────┘
                            │                      │
                            ▼                      │
                     ┌──────────────┐              │
                     │  PostgreSQL  │              │
                     │  (Txs + Audit)│             │
                     └──────────────┘              │
                            │                      │
                            │                      ▼
                     ┌──────────────┐      ┌─────────────┐
                     │    Redis     │      │  Callback   │
                     │  (Asynq)    │◀─────│  Endpoint   │
                     └──────┬───────┘      └─────────────┘
                            │
                            ▼
                     ┌──────────────┐      ┌─────────────┐
                     │   Worker     │─────▶│   Tenant    │
                     │  (Processor) │      │  Webhook    │
                     └──────────────┘      └─────────────┘
```

### Key Features

- **Security First**: SSL verification, internal auth, IP allowlist, request size limits
- **Non-blocking Callbacks**: Immediate 200 OK to Safaricom, queue-based processing
- **State Machine**: Strict transaction status transitions (PENDING → COMPLETED/FAILED)
- **Money Safety**: `shopspring/decimal` for all amounts, never float64
- **Idempotency**: UUID-based deduplication prevents duplicate charges
- **Webhook Reliability**: Exponential backoff retries with full audit trail
- **Horizontal Scaling**: Separate API and worker processes

## Project Structure

```
mpesa_gateway/
├── cmd/
│   ├── api/main.go           # HTTP API server entry point
│   └── worker/main.go        # Background worker entry point
├── internal/
│   ├── config/               # Configuration management (koanf)
│   ├── database/             # PostgreSQL connection pool (pgx)
│   ├── middleware/           # HTTP middleware (auth, IP filter, limits)
│   ├── models/               # Domain models and state machine
│   ├── mpesa/                # M-Pesa token service & helpers
│   ├── payment/              # Payment business logic (STK Push)
│   ├── queue/                # Redis queue setup (Asynq)
│   ├── server/               # Chi router configuration
│   ├── transport/http/       # HTTP handlers
│   └── worker/               # Callback processor & webhook delivery
├── migrations/
│   └── 001_initial_schema.sql
├── Dockerfile
├── docker-compose.yml
├── .env.example
└── README.md
```

## Prerequisites

- Go 1.22+
- PostgreSQL 13+
- Redis 6+
- Safaricom M-Pesa Developer Account ([Daraja Portal](https://developer.safaricom.co.ke/))

## Quick Start

### 1. Clone and Setup

```bash
git clone <repository-url>
cd mpesa_gateway

# Copy environment template
cp .env.example .env

# Edit .env with your Safaricom credentials
nano .env
```

### 2. Database Setup

```bash
# Start PostgreSQL (or use docker-compose)
docker run --name mpesa_postgres \
  -e POSTGRES_PASSWORD=mpesa_password \
  -e POSTGRES_DB=mpesa_gateway \
  -p 5432:5432 -d postgres:15-alpine

# Run migrations
psql -h localhost -U postgres -d mpesa_gateway -f migrations/001_initial_schema.sql
```

### 3. Install Dependencies

```bash
go mod download
```

### 4. Run Locally

```bash
# Terminal 1: Start Redis
docker run --name mpesa_redis -p 6379:6379 -d redis:7-alpine

# Terminal 2: Start API server
go run cmd/api/main.go

# Terminal 3: Start worker (optional, API runs worker internally)
go run cmd/worker/main.go
```

### 5. Using Docker Compose (Recommended)

```bash
# Start all services
docker-compose up -d

# View logs
docker-compose logs -f api worker

# Stop services
docker-compose down
```

## Configuration

All configuration is via environment variables with `MPESA_` prefix:

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `MPESA_SERVER_PORT` | No | 8080 | HTTP server port |
| `MPESA_DATABASE_URL` | Yes | - | PostgreSQL connection string |
| `MPESA_REDIS_URL` | Yes | - | Redis connection string |
| `MPESA_INTERNAL_SECRET` | Yes | - | Secret for X-Internal-Secret header |
| `MPESA_SAFARICOM_CONSUMER_KEY` | Yes | - | Safaricom Consumer Key |
| `MPESA_SAFARICOM_CONSUMER_SECRET` | Yes | - | Safaricom Consumer Secret |
| `MPESA_SAFARICOM_PASSKEY` | Yes | - | Safaricom STK Push Passkey |
| `MPESA_SAFARICOM_SHORT_CODE` | Yes | - | Business shortcode |
| `MPESA_SAFARICOM_CALLBACK_URL` | Yes | - | Public URL for callbacks |
| `MPESA_SAFARICOM_IPS` | No | - | Comma-separated Safaricom IPs |
| `MPESA_WORKER_CONCURRENCY` | No | 10 | Worker pool size |

See [.env.example](.env.example) for full configuration.

## API Endpoints

### POST /initiate

Initiates an STK Push payment.

**Headers:**
- `X-Internal-Secret`: Your internal authentication secret
- `Content-Type`: application/json

**Request:**
```json
{
  "amount": "100",
  "phone": "254712345678",
  "webhook_url": "https://your-app.com/webhook",
  "idempotency_key": "550e8400-e29b-41d4-a716-446655440000"
}
```

**Response (201 Created):**
```json
{
  "transaction_id": "7f8c9d1e-2a3b-4c5d-6e7f-8g9h0i1j2k3l",
  "status": "PENDING"
}
```

**Validation:**
- `amount`: Required, numeric, > 0
- `phone`: Required, exactly 12 digits, format `254XXXXXXXXX`
- `webhook_url`: Required, valid URL
- `idempotency_key`: Required, valid UUIDv4

### POST /callback

Receives M-Pesa callbacks (called by Safaricom).

**Security:** IP filtered to Safaricom IPs only.

**Response:** Always `200 OK` (queued for processing)

### GET /health

Health check endpoint.

**Response:**
```json
{
  "status": "ok",
  "database": "up"
}
```

## Webhook Payload

Your `webhook_url` will receive POST requests with this payload:

```json
{
  "transaction_id": "7f8c9d1e-2a3b-4c5d-6e7f-8g9h0i1j2k3l",
  "status": "COMPLETED",
  "amount": "100",
  "phone": "254712345678",
  "metadata": {
    "Amount": 100,
    "MpesaReceiptNumber": "OEI2AK3ZQO",
    "TransactionDate": 20240111135500,
    "PhoneNumber": "254712345678"
  },
  "timestamp": "2024-01-11T10:55:00Z"
}
```

**Headers:**
- `X-Signature`: HMAC-SHA256 signature for verification
- `Content-Type`: application/json

**Retry Policy:**
- Attempts: 4 (1min, 5min, 15min, 1hr intervals)
- Status: 2xx = success, others retry
- Timeout: 10 seconds per attempt

## Security

### Authentication

- **Internal Auth**: All `/initiate` requests require `X-Internal-Secret` header
- **Constant-time**: Uses `crypto/subtle` to prevent timing attacks

### IP Filtering

- **Safaricom IPs**: `/callback` endpoint validates source IP
- **CIDR Support**: Accepts individual IPs or CIDR ranges
- **Disable in Dev**: Empty `MPESA_SAFARICOM_IPS` allows all (dev only)

### SSL/TLS

- **Enforced**: All HTTP clients enforce SSL verification
- **Min TLS 1.2**: Configured transport requires TLS 1.2+

### Input Validation

- **go-playground/validator**: Struct field validation
- **Phone Format**: Regex validation `^254[0-9]{9}$`
- **UUIDs**: Strict UUIDv4 validation
- **Size Limits**: Max 1MB request body on callbacks

### Money Handling

- **Never float64**: All amounts use `shopspring/decimal`
- **Database**: `DECIMAL(20,2)` columns
- **Safaricom**: Amounts sent as integer (no decimals)

## State Machine

Transaction status transitions are strictly enforced:

```
PENDING ──▶ COMPLETED
   │
   └──────▶ FAILED
```

- No backward transitions
- No re-processing of terminal states (COMPLETED/FAILED)
- Database update with WHERE clause includes current state

## Monitoring

### Logs

Structured logging with timestamps and file/line numbers:

```
2024/01/11 13:55:00 api.go:45: Starting HTTP server on :8080
2024/01/11 13:55:15 processor.go:78: Processing callback for CheckoutRequestID: ws_CO_11012024135500
2024/01/11 13:55:16 processor.go:125: Transaction 7f8c9d1e updated to status: COMPLETED
```

### Database Queries

Check transaction status:
```sql
SELECT internal_transaction_id, status, amount, phone, created_at, updated_at
FROM transactions
WHERE internal_transaction_id = '7f8c9d1e-2a3b-4c5d-6e7f-8g9h0i1j2k3l';
```

Webhook delivery audit:
```sql
SELECT attempt_number, success, response_status_code, response_time_ms, attempted_at
FROM webhook_attempts
WHERE transaction_id = (SELECT id FROM transactions WHERE internal_transaction_id = '...');
```

## Deployment

### Build Binaries

```bash
# API server
go build -o bin/api cmd/api/main.go

# Worker
go build -o bin/worker cmd/worker/main.go
```

### Docker

```bash
# Build image
docker build -t mpesa-gateway:latest .

# Run API
docker run -p 8080:8080 --env-file .env mpesa-gateway:latest ./api

# Run Worker
docker run --env-file .env mpesa-gateway:latest ./worker
```

### Production Checklist

- [ ] Set strong `MPESA_INTERNAL_SECRET` (min 32 chars)
- [ ] Use production Safaricom URLs (not sandbox)
- [ ] Configure `MPESA_SAFARICOM_IPS` with real Safaricom IPs
- [ ] Enable PostgreSQL SSL (`sslmode=require`)
- [ ] Set up database backups
- [ ] Configure Redis persistence
- [ ] Use HTTPS for `MPESA_SAFARICOM_CALLBACK_URL`
- [ ] Set up monitoring/alerting for failed webhooks
- [ ] Review and adjust `MPESA_DB_MAX_CONNS` based on load
- [ ] Scale workers horizontally if queue builds up

## Testing

### Manual Testing

```bash
# Health check
curl http://localhost:8080/health

# Initiate payment
curl -X POST http://localhost:8080/initiate \
  -H "Content-Type: application/json" \
  -H "X-Internal-Secret: your-secret" \
  -d '{
    "amount": "10",
    "phone": "254712345678",
    "webhook_url": "https://webhook.site/unique-url",
    "idempotency_key": "550e8400-e29b-41d4-a716-446655440000"
  }'
```

### Simulate Callback

```bash
curl -X POST http://localhost:8080/callback \
  -H "Content-Type: application/json" \
  -d '{
    "Body": {
      "stkCallback": {
        "MerchantRequestID": "29115-34620561-1",
        "CheckoutRequestID": "ws_CO_11012024135500",
        "ResultCode": 0,
        "ResultDesc": "The service request is processed successfully.",
        "CallbackMetadata": {
          "Item": [
            {"Name": "Amount", "Value": 10},
            {"Name": "MpesaReceiptNumber", "Value": "OEI2AK3ZQO"},
            {"Name": "TransactionDate", "Value": 20240111135500},
            {"Name": "PhoneNumber", "Value": 254712345678}
          ]
        }
      }
    }
  }'
```

## Troubleshooting

### "Unauthorized" on /initiate

- Check `X-Internal-Secret` header matches `MPESA_INTERNAL_SECRET`
- Ensure no extra whitespace in secret

### "Forbidden" on /callback

- Verify source IP is in `MPESA_SAFARICOM_IPS`
- For dev, set `MPESA_SAFARICOM_IPS=""` to disable filtering

### "STK Push failed"

- Check Safaricom credentials (Consumer Key, Secret, Passkey)
- Verify `MPESA_SAFARICOM_SHORT_CODE` is correct
- Ensure using correct API URLs (sandbox vs production)
- Check token service logs for auth failures

### Webhook not delivered

- Query `webhook_attempts` table for errors
- Check tenant webhook URL is accessible
- Verify webhook endpoint accepts POST with JSON
- Review `response_status_code` and `error_message` columns

## License

Proprietary - All Rights Reserved

## Support

For issues or questions, contact: [your-email@example.com]

---

**Built with ❤️ using boring, battle-tested technology.**
