# M-Pesa Payment Gateway Microservice

M-Pesa Payment Gateway Microservice is a production-grade, security-hardened service for integrating Safaricom’s M-Pesa STK Push. Built with Go and leveraging the latest Daraja API 3.0, it is designed for reliability, maintainability, and seamless integration into applications.
## TL;DR - Quick Start

```bash
# 1. Clone and setup
git clone <repository-url> && cd mpesa_gateway
make setup

# 2. Edit .env with your Safaricom credentials
nano .env  # Add your Consumer Key, Secret, Passkey, etc.

# 3. Start everything
make up

# ✓ Done! API running at http://localhost:8081
```

**What you get:**
- ✅ PostgreSQL (auto-migrated)
- ✅ Redis (queue management)
- ✅ API Server (port 8081)
- ✅ Background Worker (callback processing)

**Useful commands:**
- `make logs` - View logs
- `make ps` - Service status
- `make down` - Stop services
- `make help` - All commands

**Need details?** Read on below ⬇️

---


### Key Features

- **Security First**: SSL verification, internal auth, IP allowlist, request size limits
- **Non-blocking Callbacks**: Immediate 200 OK to Safaricom, queue-based processing
- **State Machine**: Strict transaction status transitions (PENDING → COMPLETED/FAILED)
- **Money Safety**: `shopspring/decimal` for all amounts, never float64
- **Idempotency**: UUID-based deduplication prevents duplicate charges
- **Webhook Reliability**: Exponential backoff retries with full audit trail
- **Horizontal Scaling**: Separate API and worker processes


## Prerequisites

- **Docker** and **Docker Compose** ([Install Docker](https://docs.docker.com/get-docker/))
- **Safaricom M-Pesa Developer Account** ([Daraja Portal](https://developer.safaricom.co.ke/))

> **Local Development Only**: Go 1.22+, PostgreSQL 13+, Redis 6+ (only if running without Docker)

## Quick Start (Recommended)

### One-Command Setup

```bash
# 1. Clone the repository
git clone <repository-url>
cd mpesa_gateway

# 2. Setup environment
make setup

# 3. Edit .env with your Safaricom credentials
nano .env  # or use your preferred editor

# 4. Start all services
make up

# ✓ Done! API is now running at http://localhost:8081
```

That's it! The setup includes:
- ✅ PostgreSQL with auto-migration
- ✅ Redis for queue management
- ✅ API server on port 8081
- ✅ Background worker for callbacks

### Detailed Docker Compose Setup

#### 1. Clone and Configure

```bash
git clone <repository-url>
cd mpesa_gateway

# Copy environment template
cp .env.example .env
```

#### 2. Edit Environment Variables

Open `.env` and configure your Safaricom credentials:

```bash
# Required: Get these from https://developer.safaricom.co.ke/
MPESA_SAFARICOM_CONSUMER_KEY=your_consumer_key_here
MPESA_SAFARICOM_CONSUMER_SECRET=your_consumer_secret_here
MPESA_SAFARICOM_PASSKEY=your_passkey_here
MPESA_SAFARICOM_SHORT_CODE=174379  # Your business short code (Currently used default)

# Required: Your public callback URL (must be accessible from Safaricom)
MPESA_SAFARICOM_CALLBACK_URL=https://your-domain.com/callback

# Optional: Change internal secret for production
MPESA_INTERNAL_SECRET=change-this-to-a-strong-random-secret
```

#### 3. Start Services

```bash
# Start all services (PostgreSQL, Redis, API, Worker)
docker compose up -d

# Or use Makefile shortcut
make up
```

#### 4. Verify Setup

```bash
# Check service status
make ps

# View logs
make logs

# Test health endpoint
curl http://localhost:8081/health
```

### Useful Commands

```bash
make help        # Show all available commands
make up          # Start all services
make down        # Stop all services
make restart     # Restart services
make ps          # Show service status
make logs        # Follow logs
make shell-api   # Shell into API container
make shell-db    # Shell into database
```

### Port Mappings

The Docker containers use non-standard ports to avoid conflicts with locally running services:

| Service | Host Port | Container Port | Access URL |
|---------|-----------|----------------|------------|
| API | 8081 | 8080 | http://localhost:8081 |
| PostgreSQL | 5433 | 5432 | localhost:5433 |
| Redis | 6380 | 6379 | localhost:6380 |

**Why non-standard ports?**
- Avoids conflicts with local PostgreSQL (5432), Redis (6379), or other APIs (8080)
- Allows running both Docker and local services simultaneously
- Enables hybrid development (e.g., local Go app + Docker databases)

**Example: Connect to PostgreSQL from host**
```bash
psql -h localhost -p 5433 -U mpesa -d mpesa_gateway
# or
make shell-db  # Shortcut to connect
```


## Alternative: Local Development (Without Docker)

If you prefer to run services locally without Docker:

### 1. Start Dependencies

```bash
# Terminal 1: Start PostgreSQL
docker run --name mpesa_postgres \
  -e POSTGRES_USER=mpesa \
  -e POSTGRES_PASSWORD=mpesa_password \
  -e POSTGRES_DB=mpesa_gateway \
  -p 5432:5432 -d postgres:15-alpine

# Run migrations
psql -h localhost -U mpesa -d mpesa_gateway -f migrations/001_initial_schema.sql

# Terminal 2: Start Redis
docker run --name mpesa_redis -p 6379:6379 -d redis:7-alpine
```

### 2. Configure Environment

```bash
cp .env.example .env
nano .env  # Edit with local database URLs
```

Update `.env` for local setup:
```bash
MPESA_DATABASE_URL=postgres://mpesa:mpesa_password@localhost:5432/mpesa_gateway?sslmode=disable
MPESA_REDIS_URL=redis://localhost:6379/0
```

### 3. Run Application

```bash
# Terminal 3: Start API server
go run cmd/api/main.go

# Terminal 4: Start worker (optional)
go run cmd/worker/main.go
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
The services will be available at:
- **API**: http://localhost:8081
- **PostgreSQL**: localhost:5433
- **Redis**: localhost:6380

```bash
# Health check
curl http://localhost:8081/health

# Initiate payment
curl -X POST http://localhost:8081/initiate \
  -H "Content-Type: application/json" \
  -H "X-Internal-Secret: your-secret" \
  -d '{
    "amount": "10",
    "phone": "254712345678",
    "webhook_url": "https://webhook.site/unique-url",
    "idempotency_key": "550e8400-e29b-41d4-a716-446655440000"
  }'
```

# Simulate Callback
curl -X POST http://localhost:8081/callback \
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

### Docker Compose Issues

#### Services fail to start

```bash
# Check service logs
make logs

# or for specific service
docker compose logs postgres
docker compose logs redis
docker compose logs api
```

#### Port already in use (e.g., 5432, 6379, 8080)

```bash
# Check what's using the port
sudo lsof -i :5432  # for PostgreSQL
sudo lsof -i :6379  # for Redis
sudo lsof -i :8080  # for API

# Option 1: Stop the conflicting service
sudo systemctl stop postgresql  # if local PostgreSQL is running

# Option 2: Change ports in docker-compose.yml
# Edit docker-compose.yml and change port mappings, e.g., "5433:5432"
```

#### Database migrations didn't run

```bash
# Check if migrations ran
make shell-db
# Inside PostgreSQL:
\dt  # List tables - should see transactions and webhook_attempts

# If tables don't exist, manually run migrations
docker exec -i mpesa_postgres psql -U mpesa -d mpesa_gateway < migrations/001_initial_schema.sql
```

#### .env file not loaded

```bash
# Verify .env exists
ls -la .env

# Restart services to reload environment
make restart

# Check if environment variables loaded correctly
docker exec mpesa_api env | grep MPESA
```

#### Reset everything and start fresh

```bash
# Stop and remove all containers, networks, and volumes
docker compose down -v

# Remove any orphaned containers
docker compose rm -f

# Start fresh
make up
```

## General Troubleshooting

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

---

