# Instagram Auto-DM System

A production-ready Go application that automatically sends direct messages on Instagram when someone comments on your post with specific keywords.

## Features

✅ **Webhook Integration** - Receives Instagram comment events in real-time  
✅ **Keyword Detection** - Configurable keywords to trigger automatic DMs  
✅ **Delayed Messaging** - Sends DM after configurable delay (respects 24-hour messaging rules)  
✅ **Duplicate Prevention** - Ensures only one DM per user per post  
✅ **Retry Logic** - Exponential backoff for failed API calls  
✅ **Database Logging** - All DM sends are logged in PostgreSQL  
✅ **Production Ready** - Docker support, proper error handling, structured logging  

## Architecture

```
Instagram Comment → Meta Webhook → /webhook POST → Parse Comment
                                         ↓
                              Check Keyword Match
                                         ↓
                              Duplicate Check (DB)
                                         ↓
                           Queue Job (dmQueue Channel)
                                         ↓
                           Background Worker Picks Up
                                         ↓
                              Wait DM_DELAY (1+ seconds)
                                         ↓
                        Call Instagram Messaging API
                                         ↓
                           Log Result to DB
```

## Prerequisites

- Go 1.23+
- PostgreSQL 12+
- Instagram Business Account connected to a Facebook App
- Long-lived Instagram Access Token with messaging permissions

## Quick Start

### 1. Clone & Setup

```bash
git clone https://github.com/yourusername/instagram-autodm.git
cd instagram-autodm
```

### 2. Install Dependencies

```bash
go mod download
```

### 3. Configure Environment

Copy `.env.example` to `.env`:

```bash
cp .env.example .env
```

Edit `.env` with your values:

```env
DATABASE_URL=postgres://user:password@localhost:5432/instagram_autodm?sslmode=disable
IG_BUSINESS_ID=17841478592862410
ACCESS_TOKEN=IGAFmHDWAZCZBf5BZAFQ0d0thMVdtVVl2dU1wbmtJMzBMOGxCeHVtV1VJMWRa...
VERIFY_TOKEN=your_webhook_verify_token_here
KEYWORDS=help,dm,info
DM_MESSAGE=Hey! Thanks for your comment 👋
DM_DELAY=1s
```

### 4. Start PostgreSQL

Ensure PostgreSQL is running:

```bash
# macOS with Homebrew
brew services start postgresql

# Windows - Start the PostgreSQL service
# Linux
sudo systemctl start postgresql
```

### 5. Run the Application

```bash
go run .
```

Expected output:
```
2025/11/15 16:13:46 ✅ Database connected
2025/11/15 16:13:46 🚀 Instagram Auto-DM Server running on port 8080
2025/11/15 16:13:46 📌 Keywords: [help dm info]
```

### 6. Test Webhook Verification

```bash
curl -X GET "http://localhost:8080/webhook?hub.mode=subscribe&hub.verify_token=your_webhook_verify_token_here&hub.challenge=test_challenge_123"
```

Expected response: `test_challenge_123`

## Meta App Setup

### Step 1: Create Facebook App

1. Go to [Facebook Developers](https://developers.facebook.com)
2. Create a new app → Choose "Business" type
3. Add the **Instagram** product

### Step 2: Configure Permissions

In your app, request these permissions:
- `instagram_basic`
- `instagram_manage_messages`
- `instagram_manage_comments`

### Step 3: Connect Instagram Business Account

1. Go to Settings → Basic
2. Connect your Instagram Business Account
3. Link to your Facebook Page

### Step 4: Get Long-Lived Access Token

```bash
# 1. Get short-lived token from Graph API Explorer
# https://developers.facebook.com/tools/explorer

# 2. Exchange for long-lived token (valid 60 days)
curl -X GET "https://graph.instagram.com/oauth/access_token?grant_type=ig_refresh_token&access_token=SHORT_LIVED_TOKEN"
```

Or use this endpoint:
```bash
curl -X GET "https://graph.facebook.com/oauth/access_token?client_id=YOUR_APP_ID&client_secret=YOUR_APP_SECRET&grant_type=fb_exchange_token&fb_exchange_token=SHORT_LIVED_TOKEN"
```

### Step 5: Configure Webhook

1. In your app → Settings → Webhooks
2. Add webhook URL: `https://your-domain.com/webhook`
3. Add verify token (set same as `VERIFY_TOKEN` in `.env`)
4. Subscribe to field: `comments` and `messaging` (if needed)
5. Click Subscribe

**Test verification:** Facebook will send a GET request and expect your server to respond with the challenge token.

## Environment Variables

| Variable | Description | Example |
|----------|-------------|---------|
| `DATABASE_URL` | PostgreSQL connection string | `postgres://user:pass@localhost:5432/db?sslmode=disable` |
| `IG_BUSINESS_ID` | Your Instagram Business Account ID | `17841478592862410` |
| `ACCESS_TOKEN` | Long-lived Instagram API token | `IGAFmHDWAZCZ...` |
| `VERIFY_TOKEN` | Webhook verification token (you set this) | `my_secret_token` |
| `KEYWORDS` | Comma-separated keywords to trigger DM | `help,dm,info,send` |
| `DM_MESSAGE` | Message text to send | `Thanks for commenting!` |
| `DM_DELAY` | Delay before sending DM | `1s` (1 second), `60s` (1 minute) |
| `PORT` | Server port | `8080` |
| `MAX_RETRIES` | Max retry attempts on API failure | `3` |

## Database Schema

### dm_logs table

Tracks all DM sends:

```sql
CREATE TABLE dm_logs (
    id SERIAL PRIMARY KEY,
    user_id VARCHAR(255) NOT NULL,
    post_id VARCHAR(255) NOT NULL,
    comment_id VARCHAR(255) NOT NULL,
    sent_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    status VARCHAR(50) NOT NULL,
    retry_count INTEGER DEFAULT 0,
    error_message TEXT,
    UNIQUE(user_id, post_id)  -- Prevents duplicate DMs per user per post
);
```

## Deployment

### Docker

Build and run with Docker:

```bash
# Build
docker build -t instagram-autodm:latest .

# Run
docker run -d \
  --name instagram-autodm \
  -p 8080:8080 \
  -e DATABASE_URL="postgres://user:pass@db:5432/db" \
  -e IG_BUSINESS_ID="17841478592862410" \
  -e ACCESS_TOKEN="IGAFmHDWAZCZ..." \
  -e VERIFY_TOKEN="my_token" \
  -e KEYWORDS="help,dm,info" \
  instagram-autodm:latest
```

### Render (Recommended)

1. Push code to GitHub
2. Create new **Web Service** on Render
3. Connect GitHub repo
4. Add environment variables
5. Add Postgres database add-on
6. Deploy

### Railway

1. Push code to GitHub
2. Create new project on Railway
3. Add Postgres plugin
4. Deploy GitHub repo
5. Set environment variables
6. Configure webhook URL

### Heroku

```bash
# Login
heroku login

# Create app
heroku create your-app-name

# Add Postgres
heroku addons:create heroku-postgresql:hobby-dev

# Set env vars
heroku config:set ACCESS_TOKEN=IGAFmHDWAZCZ...
heroku config:set IG_BUSINESS_ID=17841478592862410
heroku config:set VERIFY_TOKEN=my_token

# Deploy
git push heroku main

# View logs
heroku logs --tail
```

## API Endpoints

### GET /webhook
Webhook verification handshake

**Parameters:**
- `hub.mode=subscribe`
- `hub.challenge=<challenge_string>`
- `hub.verify_token=<your_verify_token>`

**Response:** Challenge string (200 OK)

### POST /webhook
Receives comment events from Instagram

**Body:** Instagram webhook payload

**Response:** 200 OK

### GET /health
Health check endpoint

**Response:**
```json
{
  "status": "healthy",
  "queue_size": 0,
  "keywords": ["help", "dm", "info"]
}
```

## Troubleshooting

### "DB ping failed"
- Ensure PostgreSQL is running: `psql postgres://user:pass@localhost/db`
- Check `DATABASE_URL` is correct in `.env`

### "WEBHOOK VERIFIED" not logged
- Verify `VERIFY_TOKEN` matches what's set in Meta App Dashboard
- Check webhook URL is publicly accessible (not localhost)

### DM not being sent
- Check logs for API errors: `📥 Response Status: XXX`
- Verify `ACCESS_TOKEN` is valid and long-lived
- Ensure `IG_BUSINESS_ID` is correct
- Check Instagram account has messaging permissions approved

### Rate Limited (429)
- Wait and retry - backoff is exponential
- Consider increasing `DM_DELAY` to space out messages
- Check Instagram API rate limits in dashboard

## Advanced Configuration

### Retry Backoff Strategy

Failed DM sends are retried with exponential backoff:
- Attempt 1: Immediate
- Attempt 2: 2 seconds
- Attempt 3: 4 seconds
- Attempt 4: 8 seconds (and so on...)
- After 3 retries: Marked as failed

To customize, edit `RetryBackoffBase` in `main.go`.

### Custom DM Message with Variables

Extend `sendDM()` to include user info:

```go
message := fmt.Sprintf("Hey @%s! Thanks for commenting '%s'", username, commentText)
```

### Multiple Keywords

Add keywords as comma-separated in `.env`:

```env
KEYWORDS=help,support,free,discount,dm,info,send
```

## Production Checklist

- [ ] PostgreSQL database created and backed up regularly
- [ ] `ACCESS_TOKEN` stored in secrets manager (not `.env`)
- [ ] Webhook URL uses HTTPS with valid certificate
- [ ] Rate limiting configured (consider adding Redis)
- [ ] Monitoring/alerting set up for failed DMs
- [ ] Regular token refresh process implemented
- [ ] Database connection pooling optimized
- [ ] Logs aggregated (Datadog, Sentry, etc.)

## Support

For issues, check:
1. Application logs for error messages
2. Database `dm_logs` table for failed sends
3. Instagram API documentation for permission errors
4. Meta Graph API Explorer to test endpoints directly

## License

MIT

## Contributing

Pull requests welcome! Please test thoroughly before submitting.
