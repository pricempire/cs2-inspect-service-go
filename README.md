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
- SOCKS5 proxy support for bot connections
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

   # Proxy configuration (optional)
   # PROXY_ENABLED=true
   # PROXY_URL=socks5://username:password@proxy.example.com:1080
   ```

   </details>

3. Create an `accounts.txt` file with your Steam accounts in the format:

   <details>
   <summary>Account format</summary>

   ```
   username:password
   username2:password2
   ```

   For accounts with proxies, you can specify them per account:

   ```
   username:password:socks5://proxy.example.com:1080
   username2:password2:socks5://proxy2.example.com:1080
   ```

   </details>

4. Set up the PostgreSQL database:

   <details>
   <summary>Database setup SQL</summary>

   ```sql
   CREATE DATABASE cs2_inspect;

   CREATE TABLE asset (
       unique_id VARCHAR(64) NOT NULL,
       asset_id BIGINT NOT NULL,
       ms BIGINT NOT NULL,
       d VARCHAR(32) NOT NULL,
       paint_seed SMALLINT,
       paint_index SMALLINT,
       paint_wear DOUBLE PRECISION,
       quality SMALLINT,
       custom_name VARCHAR(64),
       def_index SMALLINT,
       origin SMALLINT,
       rarity SMALLINT,
       quest_id SMALLINT,
       reason SMALLINT,
       music_index SMALLINT,
       ent_index SMALLINT,
       is_stattrak BOOLEAN DEFAULT FALSE,
       is_souvenir BOOLEAN DEFAULT FALSE,
       stickers JSONB,
       keychains JSONB,
       killeater_score_type SMALLINT,
       killeater_value INTEGER,
       pet_index SMALLINT,
       inventory BIGINT,
       drop_reason SMALLINT,
       created_at TIMESTAMP NOT NULL,
       updated_at TIMESTAMP NOT NULL,
       PRIMARY KEY (asset_id, ms, d)
   );

   CREATE INDEX asset_unique_id ON asset (unique_id);
   CREATE INDEX asset_paint_details ON asset (paint_seed, paint_index, paint_wear);
   CREATE INDEX asset_item_details ON asset (def_index, quality, rarity, origin);

   CREATE TABLE history (
       id SERIAL PRIMARY KEY,
       unique_id VARCHAR(64) NOT NULL,
       asset_id BIGINT NOT NULL,
       prev_asset_id BIGINT,
       owner VARCHAR(64) NOT NULL,
       prev_owner VARCHAR(64),
       d VARCHAR(32) NOT NULL,
       stickers JSONB,
       keychains JSONB,
       prev_stickers JSONB,
       prev_keychains JSONB,
       type VARCHAR(32) NOT NULL,
       created_at TIMESTAMP NOT NULL,
       updated_at TIMESTAMP NOT NULL,
       UNIQUE (asset_id, unique_id)
   );

   CREATE INDEX history_unique_id ON history (unique_id);
   CREATE INDEX history_asset_id ON history (asset_id);
   CREATE INDEX history_type ON history (type);
   ```

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
	"bots": [
		{
			"username": "bot1",
			"connected": true,
			"loggedOn": true,
			"ready": true,
			"busy": false
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

## Bot Management

The service manages multiple Steam bots to handle concurrent inspect requests.

<details>
<summary>Bot management details</summary>

### Bot States

Bots can be in one of the following states:

- `DISCONNECTED`: Bot is not connected to Steam
- `CONNECTING`: Bot is connecting to Steam
- `CONNECTED`: Bot is connected to Steam but not logged in
- `LOGGING_IN`: Bot is logging in to Steam
- `LOGGED_IN`: Bot is logged in to Steam but not ready for Game Coordinator requests
- `READY`: Bot is ready to handle Game Coordinator requests
- `BUSY`: Bot is currently handling a Game Coordinator request

### Bot Selection

When a request is received, the service selects an available bot (in the `READY` state) to handle the request. If no bots are available, the request fails with an error.

### Automatic Reconnection

The service automatically reconnects bots that go down or become unresponsive. It also periodically checks the Game Coordinator connection and reconnects if necessary.

### Manual Reconnection

The service provides an endpoint to manually trigger a reconnection for all bots, which can be useful if the bots are stuck in an invalid state.

</details>

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

## Proxy Support

The service supports using SOCKS5 proxies for bot connections to Steam. This can be useful to:

- Avoid IP-based rate limits from Steam
- Distribute connections across different regions
- Improve reliability by having fallback connection methods

Proxies can be configured in two ways:

1. **Global proxy configuration** in the `.env` file:

   ```
   PROXY_ENABLED=true
   PROXY_URL=socks5://username:password@proxy.example.com:1080
   ```

2. **Per-account proxy configuration** in the `accounts.txt` file:

   ```
   username:password:socks5://proxy.example.com:1080
   username2:password2:socks5://proxy2.example.com:1080
   ```

When both global and per-account proxies are configured, the per-account proxy takes precedence.

> **Note:** The service only supports SOCKS5 proxies. HTTP proxies are not supported.

## Troubleshooting

<details>
<summary>Common issues and solutions</summary>

### Bot connection issues

If bots are having trouble connecting to the Game Coordinator:

1. Check the bot's status using the `/health` endpoint
2. Try manually reconnecting the bot using the `/reconnect` endpoint with the `force=true` parameter
3. Verify that your Steam accounts have CS2 access and are not VAC banned
4. If using proxies, ensure they are working correctly and can connect to Steam servers

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
