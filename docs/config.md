## Configuration Management

The service manages its application state using the following packages:
* `"github.com/gookit/config/v2"` — Core configuration engine.
* `"github.com/gookit/config/v2/yaml"` — YAML driver extension for parsing config files.

### Configuration File Path

By default, the application looks for a configuration file in its standard directory. You can explicitly override and pass a custom configuration path using the `-config` CLI flag:

```bash
./notifier -config /path/to/config.yaml
```

### Environment Variables & Secrets

To maintain security best practices and keep production configurations clean:
* **Environment Variables**: Sensitive values or application overrides can be injected directly via system environment variables (`ENV`).
* **Dotenv Support**: For local development or isolated containers, the service automatically loads secrets from a `.env` file located in the root directory.

### Multiple delivery profiles

Keys under `providers.providers` identify configured delivery profiles. Add a separate key for each account or default sender, for example `sales_mail` and `support_mail`, both with `provider: smtp` but different `params.from` and credentials as required by the SMTP server. Copy the existing worker/breaker settings for each instance. The request selects the profile through `transport`; `sender_id` remains the initiator.

SMS originators depend on the plugin and provider account. For a plugin supporting `params.sender`, configure each approved originator in its own instance. A Beeline account may determine the originator itself; adding a profile cannot grant an unregistered sender name. SMTP also supports a per-message From override, subject to the SMTP server's rules; the catalog reports only the configured default.

`GET /api/v1/transports` lists the configured profile codes and provider types without credentials. Explicit SMTP `from` and SMSC `sender` defaults are included when set. A missing `sender` is not an error. No new sender database or request field is needed.

### Example

```yaml
# ==============================================================================
# NOTIFICATION SERVICE CONFIGURATION TEMPLATE
# ==============================================================================

# HTTP Server Configuration
http:
  # Network address and port for the HTTP API gateway (e.g., "0.0.0.0:8080")
  addr: ${APP_HTTP_ADDR}
  # Toggle to serve Swagger UI documentation endpoint (/swagger/*)
  withSwagger: false
  config:
    # Maximum duration for reading the entire incoming request body
    readTimeout: ${APP_HTTP_READ_TIMEOUT|1s}
    # Amount of time allowed to read request headers
    readHeaderTimeout: ${APP_HTTP_READ_HEADER_TIMEOUT|1s}
    # Maximum duration before timing out writes of the response
    writeTimeout: ${APP_HTTP_WRITE_TIMEOUT|2s}
  tls:
    # Path to the SSL/TLS certificate file (leave blank to disable HTTPS)
    cert: ${APP_HTTP_TLS_CERT}
    # Path to the SSL/TLS private key file
    key: ${APP_HTTP_TLS_KEY}

# Database & Migrations Configuration
db:
  # Database Connection String (Data Source Name)
  dsn: ${APP_DB_DSN}
  # SQL driver name (defaults to 'pgx' for PostgreSQL)
  driver: ${APP_DB_DRIVER|pgx}
  # Directory where database schema .sql migration files are stored
  migrationDir: ${APP_DB_MIGRATION_DIR|./migrations}

# Core Messaging Engine & Plugins Lifecycle Configuration
providers:
  # Maximum number of outgoing messages allowed to sit in the internal buffer queue
  maxBufferSize: 1000
  # Duration the engine waits to grab a notification task from the internal queue
  fetchTaskTimeout: 2s

  # HashiCorp go-plugin Registry
  # Maps a provider type alias to its respective compiled sub-process binary file path
  pluginMap:
    telegram: ./plugins/dist/providers/tg
    smtp: ./plugins/dist/providers/smtp

  # Active Provider Instances Configuration
  providers:
    # --------------------------------------------------------------------------
    # SMTP (Email) Instance Example: Gmail
    # --------------------------------------------------------------------------
    gmail:
      provider: smtp        # References the underlying plugin defined in pluginMap
      settings:
        workers:            # Worker pool for scaling concurrent SMTP dispatches
          min: 2            # Minimum active persistent background senders
          max: 6            # Maximum allowed concurrent background senders under load
        breaker:            # Circuit Breaker fail-safe metrics
          windowSize: 10m   # Time window used to aggregate failure statistics
          minRequests: 100  # Minimum request volume required to evaluate the breaker status
          failureThreshold: 0.5 # Trips the breaker if more than 50% of requests fail
          timeout: 5m       # Sleep window duration to block requests before attempting a retry
      params:               # JSON-serialized payload sent directly to the SMTP plugin's Configure method
        host: smtp.gmail.com
        port: 587
        user: user
        password: password
        from: mail@example.com

    # --------------------------------------------------------------------------
    # Telegram Messenger Bot Instance
    # --------------------------------------------------------------------------
    tg:
      provider: telegram    # References the underlying plugin defined in pluginMap
      onboarding:           # Optional phone-to-bot onboarding; disabled by default
        enabled: ${APP_MESSENGER_ONBOARDING_ENABLED|false}
        botID: "${APP_TELEGRAM_BOT_ID|}" # Must match this provider's sidecar bot ID
        smsTransport: beeline
        connectURL: https://mircli.ru/notification/connect/tg
      settings:
        workers:            # Worker pool matching strict Telegram API rate bounds
          min: 2
          max: 2
        breaker:            # Circuit Breaker tailored for aggressive rate-limiting APIs
          windowSize: 10m
          minRequests: 100
          failureThreshold: 0.3 # Strips early (30% failure rate) to protect bot API reputation
          timeout: 5m
      params:               # JSON-serialized payload sent directly to the Telegram plugin's Configure method
        token: ${TELEGRAM_BOT_TOKEN} # Safe injection placeholder for sensitive bot credentials
```

When onboarding is enabled for a Telegram or MAX profile, `botID`, `smsTransport`, and `connectURL` are required. `smsTransport` must name another configured profile. Keep `enabled` false where the matching bot sidecar and binding tables are unavailable. Bot IDs are strings so their numeric size is preserved exactly.
