# CS2 Inspect Service

A Go service for inspecting CS2 items using the Steam Game Coordinator. This service allows you to retrieve detailed information about CS2 items, including stickers, keychains, wear values, and more.

## Features

- Multiple bot support for handling concurrent inspect requests
- Automatic reconnection for bots that go down
- Health check endpoint for monitoring bot status
- Manual reconnect endpoint for triggering bot reconnection
- PostgreSQL database integration for caching inspect results
- Periodic Game Coordinator connection checks
- Item history tracking for monitoring changes in ownership, stickers, keychains, and nametags
- Support for sticker names, keychain names, and pattern information
- Float value ranking system
- Detailed wear information including min/max values
- Support for special patterns (Case Hardened, Fade, Marble Fade, Doppler phases)
- SOCKS5 and HTTP proxy support for bot connections
- Concurrent bot initialization with queue-based processing
- Automatic blacklisting of problematic accounts
- Session management with refresh token support
- Detailed progress tracking and logging
- Web interface for testing and API documentation

## Links

- [Pricempire](https://pricempire.com/) - CS2 price comparison platform
- [GitHub Repository](https://github.com/pricempire/cs2-inspect-service-go) - Source code and documentation

## Setup

### Prerequisites

- Go 1.18 or higher
- PostgreSQL database
- Steam accounts with CS2 access
- (Optional) SOCKS5 proxies for bot connections

### Installation

1. Clone the repository:

   <details>
   <summary>Clone command</summary>

   ```bash
   git clone https://github.com/pricempire/cs2-inspect-service-go.git
   cd cs2-inspect-service-go
   ```

   </details>

2. Copy `.env.example` to `.env` and configure the environment variables:

   <details>
   <summary>Environment setup</summary>

   ```bash
   cp .env.example .env
   ```

   Edit the `.env` file with your configuration:

   ```
   # Server configuration
   PORT=3000

   # Database configuration
   DB_HOST=localhost
   DB_PORT=5432
   DB_USER=postgres
   DB_PASSWORD=password
   DB_NAME=cs2_inspect

   # Steam configuration
   STEAM_API_KEY=your_steam_api_key

   # Service configuration
   REQUEST_TIMEOUT=30s
   BOT_RECONNECT_INTERVAL=5m
   MAX_CONCURRENT_REQUESTS=10
   MAX_CONCURRENT_BOTS=10

   # Proxy configuration (optional)
   # PROXY_ENABLED=true
   # PROXY_URL=socks5://username:password@proxy.example.com:1080
   # or for HTTP proxy
   # PROXY_URL=http://username:password@proxy.example.com:8080
   # For dynamic session assignment
   # PROXY_URL=http://username:password@proxy.example.com:8080?session=[session]

   # Session management
   SESSION_DIR=sessions
   BLACKLIST_PATH=blacklist.txt
   ```

   </details>

3. Create an `accounts.txt` file with your Steam accounts in the format:

   <details>
   <summary>Account format</summary>

   ```
   username:password
   username2:password2
   ```

   For accounts with additional authentication information:

   ```
   username:password:sentry_hash:shared_secret
   ```

   The `sentry_hash` and `shared_secret` are used for Steam Guard authentication.

   For accounts with proxies, you can specify them per account:

   ```
   username:password:socks5://proxy.example.com:1080
   username2:password2:http://proxy2.example.com:8080
   ```

   Full format with all options:

   ```
   username:password:sentry_hash:shared_secret:proxy_url
   ```

   </details>

4. Set up the PostgreSQL database:

   <details>
   <summary>Database setup</summary>

   The SQL initialization script is located in the `sql/init.sql` file. You can run it using:

   ```bash
   psql -U postgres -f sql/init.sql
   ```

   Or manually create the database and tables using the SQL commands in the file.

   The script will:

   - Create the `cs2_inspect` database
   - Create the `asset` table for storing item information
   - Create the `history` table for tracking item changes
   - Set up appropriate indexes for performance optimization

   </details>

5. Build and run the service:
   <details>
   <summary>Build and run commands</summary>

   ```bash
   go mod tidy
   go build
   ./cs2-inspect-service-go
   ```

   Or run directly:

   ```bash
   GOLANG_PROTOBUF_REGISTRATION_CONFLICT=warn go run .
   ```

   </details>

## API Endpoints

### Inspect Item

Retrieves detailed information about a CS2 item using its inspect link.

<details>
<summary>Endpoint details</summary>

**Request:**

```
GET /inspect?link=steam://rungame/730/...
```

Optional parameters:

- `refresh=true` - Force a refresh from the Game Coordinator instead of using cached data
- `force=true` - Force reconnect the bot to the Game Coordinator before processing the request

**Response:**

```json
{
	"success": true,
	"itemInfo": {
		"def_index": 1,
		"paint_index": 2,
		"rarity": 3,
		"quality": 4,
		"paint_wear": 0.123,
		"paint_seed": 5,
		"custom_name": "My Skin",
		"killeater_score_type": 0,
		"killeater_value": 0,
		"origin": 8,
		"quest_id": 0,
		"drop_reason": 0,
		"music_index": 0,
		"ent_index": 0,
		"pet_index": 0,
		"inventory": 0,
		"is_stattrak": true,
		"is_souvenir": false,
		"stickers": [
			{
				"id": 1,
				"wear": 0.1,
				"scale": 1.0,
				"rotation": 0.0,
				"name": "Sticker | Team Liquid | Stockholm 2021"
			}
		],
		"keychains": [
			{
				"id": 1,
				"wear": 0.0,
				"scale": 1.0,
				"rotation": 0.0,
				"name": "Baby Karat CT"
			}
		],
		"wear_name": "Factory New",
		"phase": "Phase 2",
		"market_hash_name": "★ Karambit | Doppler (Factory New)",
		"pattern": "Blue Gem",
		"min": 0.0,
		"max": 0.08,
		"rank": 1,
		"total_count": 100,
		"type": "Weapon",
		"image": "https://community.cloudflare.steamstatic.com/economy/image/-9a81dlWLwJ2UUGcVs_nsVtzdOEdtWwKGZZLQHTxDZ7I56KU0Zwwo4NUX4oFJZEHLbXH5ApeO4YmlhxYQknCRvCo04DEVlxkKgpot621FAR17P7NdTRH-t26q4SZlvD7PYTQgXtu5Mx2gv2PrdSijAWwqkVtN272JIGdJw46YVrYqVO3xLy-gJC9u5vByCBh6ygi7WGdwUKTYdRD8A"
	},
	"cached": false
}
```

</details>

### Health Check

Checks the health status of the service and its bots.

<details>
<summary>Endpoint details</summary>

**Request:**

```
GET /health
```

**Response:**

```json
{
	"status": "healthy",
	"bots": {
		"ready": 10,
		"busy": 0,
		"cooldown": 0,
		"disconnected": 0,
		"error": 0,
		"initializing": 0,
		"total": 10,
		"utilization": "0.00%"
	},
	"details": [
		{
			"username": "bot1",
			"connected": true,
			"loggedOn": true,
			"ready": true,
			"busy": false,
			"state": "ready",
			"lastUsed": "2023-06-01T12:34:56Z",
			"reconnectAttempts": 0
		},
		{
			"username": "bot2",
			"connected": true,
			"loggedOn": true,
			"ready": true,
			"busy": true
		}
	]
}
```

Possible status values:

- `healthy` - All bots are ready
- `degraded` - Some bots are ready, but not all
- `unhealthy` - No bots are ready
</details>

### Reconnect Bots

Triggers a reconnection for all bots.

<details>
<summary>Endpoint details</summary>

**Request:**

```
GET /reconnect
```

**Response:**

```json
{
	"success": true,
	"message": "Reconnect triggered for 1 bots"
}
```

</details>

### Item History

Retrieves the history of an item based on its unique ID.

<details>
<summary>Endpoint details</summary>

**Request:**

```
GET /history?uniqueId=abcd1234
```

**Response:**

```json
{
    "success": true,
    "history": [
        {
            "id": 1,
            "uniqueId": "abcd1234",
            "assetId": 12345678901,
            "prevAssetId": 12345678900,
            "owner": "76561198123456789",
            "prevOwner": "76561198987654321",
            "d": "123456789",
            "stickers": [...],
            "keychains": [...],
            "prevStickers": [...],
            "prevKeychains": [...],
            "type": "TRADE",
            "createdAt": "2023-06-01T12:34:56Z",
            "updatedAt": "2023-06-01T12:34:56Z"
        }
    ]
}
```

</details>

## Database Caching

The service uses PostgreSQL to cache inspect results. When a request is made, the service first checks if the item is already in the database. If it is, the cached result is returned immediately. If not, the service fetches the item information from the Game Coordinator and then saves it to the database for future requests.

This caching mechanism significantly improves performance for frequently requested items and reduces the load on the Steam Game Coordinator.

### Float Ranking System

The service includes a float ranking system that calculates and provides rankings for item float values. These rankings are stored in a materialized view called `rankings` and are updated periodically.

The ranking system provides:

- **Global rankings**: Where an item's float value ranks among all items in the database

  - `global_low`: Rank among items with the highest float values (most battle-scarred)
  - `global_high`: Rank among items with the lowest float values (most factory new)

- **Item-specific rankings**: Where an item's float value ranks among similar items
  - `low_rank`: Rank among items of the same type with the highest float values
  - `high_rank`: Rank among items of the same type with the lowest float values

Item-specific rankings are partitioned by:

- Paint index (skin type)
- Def index (weapon type)
- StatTrak™ status
- Souvenir status

This allows users to see how rare an item's float value is compared to other similar items, which can be valuable information for collectors and traders.

## History Tracking

The service tracks changes to items over time, including:

- Ownership changes (trades, market transactions)
- Sticker application, removal, or changes
- Keychain addition, removal, or changes
- Nametag addition or removal

Each time an item is inspected, the service compares the current state with the previous state stored in the database. If changes are detected, a new history record is created with the type of change and relevant details.

For new items that haven't been seen before, the service creates an initial history record with a type based on the item's origin (e.g., UNBOXED, DROPPED, TRADED_UP). This provides a complete history of the item from its first appearance in the system.

### History Types

<details>
<summary>List of history types</summary>

The following history types are tracked:

- `UNKNOWN`: Unknown change type
- `TRADE`: Item changed ownership through a trade
- `MARKET_BUY`: Item was purchased from the Steam Market
- `MARKET_LISTING`: Item was listed on the Steam Market
- `TRADED_UP`: Item was obtained through a trade-up contract
- `DROPPED`: Item was dropped in-game
- `PURCHASED_INGAME`: Item was purchased in-game
- `UNBOXED`: Item was unboxed from a case
- `CRAFTED`: Item was crafted
- `STICKER_APPLY`: Sticker was applied to the item
- `STICKER_REMOVE`: Sticker was removed from the item
- `STICKER_SCRAPE`: Sticker was scraped
- `STICKER_CHANGE`: Sticker was changed
- `KEYCHAIN_ADDED`: Keychain was added to the item
- `KEYCHAIN_REMOVED`: Keychain was removed from the item
- `KEYCHAIN_CHANGED`: Keychain was changed
- `NAMETAG_ADDED`: Nametag was added to the item
- `NAMETAG_REMOVED`: Nametag was removed from the item
</details>

## Schema System

The service uses a schema system to provide additional information about items, such as sticker names, keychain names, weapon names, and paint names. The schema is loaded from the CSFloat API and cached in memory.

<details>
<summary>Schema structure</summary>

The schema includes:

- **Weapons**: Information about weapons, including their names and available paints
- **Stickers**: Information about stickers, including their market hash names
- **Keychains**: Information about keychains, including their market hash names
- **Agents**: Information about agents, including their market hash names and images

The schema is used to:

1. Build market hash names for items
2. Determine wear names based on float values
3. Identify special patterns (Case Hardened, Fade, Marble Fade)
4. Provide phase names for Doppler knives
5. Add sticker and keychain names to the response
</details>

## Special Pattern Support

The service supports special patterns for certain skins, providing additional information about them.

<details>
<summary>Supported special patterns</summary>

### Doppler Phases

For Doppler knives, the service identifies the phase based on the paint index:

- Phase 1, 2, 3, 4
- Ruby
- Sapphire
- Black Pearl
- Emerald

### Case Hardened Patterns

For Case Hardened skins, the service identifies special patterns based on the paint seed:

- Blue Gems
- Scar Pattern
- Golden Booty
- And more

### Fade Percentages

For Fade skins, the service identifies the fade percentage based on the paint seed.

### Marble Fade Patterns

For Marble Fade knives, the service identifies special patterns based on the paint seed:

- Fire & Ice
- Fake Fire & Ice
- Tricolor
- And more
</details>

## Proxy Support

The service supports using SOCKS5 and HTTP proxies for bot connections to Steam. This can be useful to:

- Avoid IP-based rate limits from Steam
- Distribute connections across different regions
- Improve reliability by having fallback connection methods

Proxies can be configured in two ways:

1. **Global proxy configuration** in the `.env` file:

   ```
   PROXY_ENABLED=true
   PROXY_URL=socks5://username:password@proxy.example.com:1080
   # or for HTTP proxy
   PROXY_URL=http://username:password@proxy.example.com:8080
   ```

2. **Per-account proxy configuration** in the `accounts.txt` file:

   ```
   username:password:socks5://proxy.example.com:1080
   username2:password2:http://proxy2.example.com:8080
   ```

When both global and per-account proxies are configured, the per-account proxy takes precedence.

The service now supports both SOCKS5 and HTTP proxies:

- **SOCKS5 proxies**: Full support for TCP connections
- **HTTP proxies**: Support for CONNECT method to establish TCP tunnels

You can also use the `PROXY_URL` environment variable with a template format to automatically assign different proxies to bots:

```
PROXY_URL=http://username:password@proxy.example.com:8080?session=[session]
```

The `[session]` placeholder will be replaced with the bot's username and an index, allowing for unique session identifiers per bot.

## Enhanced Bot Management

The service includes several improvements to bot management:

### Concurrent Bot Initialization

The bot initialization process has been enhanced to support efficient concurrent initialization:

- A configurable maximum number of concurrent initializations (default: 10)
- Queue-based processing to ensure all bots are initialized without overwhelming the system
- Progress tracking and detailed logging of initialization status
- Automatic timeout handling for bots that fail to initialize within a reasonable time

You can configure the maximum concurrent initializations using the environment variable:

```
MAX_CONCURRENT_BOTS=10
```

### Improved Health Monitoring

The bot health monitoring system has been enhanced with:

- Periodic Game Coordinator connection checks
- Automatic detection and recovery of disconnected bots
- Proactive reconnection of bots with GC connection issues
- Detailed health status reporting via the `/health` endpoint
- Reconnection queue to manage bot reconnections in a controlled manner

### Session Management

The service now includes improved session management:

- Persistent storage of refresh tokens for faster reconnection
- Automatic session recovery after service restart
- Session file rotation to prevent token expiration
- Blacklisting of problematic accounts with reason tracking

## Blacklist System

The service now includes a blacklist system to automatically exclude problematic accounts:

- Accounts with invalid credentials
- Accounts requiring Steam Guard authentication
- Banned or disabled accounts
- Accounts with other persistent connection issues

The blacklist is stored in a `blacklist.txt` file and includes the reason for blacklisting:

```
username:credentials
username2:steamguard
username3:banned
```

Blacklisted accounts are automatically skipped during initialization, preventing them from causing repeated connection failures.

## Logging

The service provides detailed logging to help with debugging and monitoring.

<details>
<summary>Logging details</summary>

### Log Levels

- `ERROR`: Critical errors that prevent the service from functioning
- `WARNING`: Non-critical issues that might affect functionality
- `INFO`: General information about the service operation
- `DEBUG`: Detailed information for debugging purposes

### Log Format

Logs include:

- Timestamp
- Log level
- Message
- Additional context (if available)

Example:

```
2023-06-01 12:34:56 [INFO] Received inspect request for link: steam://rungame/730/... (refresh: false)
2023-06-01 12:34:56 [INFO] Parsed inspect link: A:12345678901 D:123456789 S:76561198123456789 M:0
2023-06-01 12:34:56 [INFO] Using bot: bot1
2023-06-01 12:34:56 [INFO] Waiting for response with timeout of 30s
2023-06-01 12:34:57 [INFO] Received response with 1024 bytes
2023-06-01 12:34:57 [INFO] Saved asset to database: abcd1234
```

</details>

## Web Interface

The service includes a web interface for testing the API and viewing documentation. When you access the root URL of the service without any parameters, it will serve an HTML page with:

- A form for submitting inspect links
- Option to force reconnect the bot to the Game Coordinator
- API documentation
- Visual and JSON views of inspection results

The web interface is accessible at the root URL of the service (e.g., `http://localhost:3000/`).

## Troubleshooting

<details>
<summary>Common issues and solutions</summary>

### Bot connection issues

If bots are having trouble connecting to the Game Coordinator:

1. Check the bot's status using the `/health` endpoint
2. Try manually reconnecting the bot using the `/reconnect` endpoint with the `force=true` parameter
3. Verify that your Steam accounts have CS2 access and are not VAC banned
4. If using proxies, ensure they are working correctly and can connect to Steam servers
5. Check the blacklist.txt file to see if accounts have been automatically blacklisted
6. Verify that your HTTP proxies support the CONNECT method if you're using HTTP proxies
7. Check the session files in the sessions directory for any issues with refresh tokens

### Bot initialization issues

If bots are not initializing properly:

1. Check the logs for any errors during the initialization process
2. Verify that the MAX_CONCURRENT_BOTS setting is appropriate for your system
3. Ensure that your proxies can handle the concurrent connection load
4. Check if Steam is experiencing login issues or maintenance
5. Try increasing the initialization timeout if bots are timing out during initialization

### Database connection issues

If the service can't connect to the database:

1. Verify that PostgreSQL is running
2. Check the database credentials in the `.env` file
3. Ensure the database and tables are created correctly
4. Check network connectivity between the service and the database

### Game Coordinator issues

If the service can't connect to the CS2 Game Coordinator:

1. Check if CS2 servers are down or under maintenance
2. Ensure your Steam accounts have CS2 access
3. Try reconnecting the bots using the `/reconnect` endpoint
4. Restart the service

### Performance issues

If the service is slow or unresponsive:

1. Increase the number of bots to handle more concurrent requests
2. Optimize database queries and indexes
3. Increase the request timeout if necessary
4. Consider scaling horizontally by running multiple instances of the service
</details>

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

This project is licensed under the MIT License - see the LICENSE file for details.
