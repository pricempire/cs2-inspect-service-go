# CS2 Inspect Service

A Go service for inspecting CS2 items using the Steam Game Coordinator.

## Features

- Multiple bot support for handling concurrent inspect requests
- Automatic reconnection for bots that go down
- Health check endpoint for monitoring bot status
- Manual reconnect endpoint for triggering bot reconnection
- PostgreSQL database integration for caching inspect results
- Periodic Game Coordinator connection checks
- Item history tracking for monitoring changes in ownership, stickers, keychains, and nametags

## Setup

1. Copy `.env.example` to `.env` and configure the environment variables:

   ```
   cp .env.example .env
   ```

2. Create an `accounts.txt` file with your Steam accounts in the format:

   ```
   username:password
   ```

3. Set up the PostgreSQL database:

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

4. Build and run the service:
   ```
   go build
   ./cs2-inspect-service-go
   ```

## API Endpoints

### Inspect Item

```
GET /inspect?link=steam://rungame/730/...
```

Response:

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
		"stickers": [
			{
				"id": 1,
				"wear": 0.1,
				"scale": 1.0,
				"rotation": 0.0
			}
		],
		"is_stattrak": true,
		"is_souvenir": false
	},
	"cached": false
}
```

### Health Check

```
GET /health
```

Response:

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
		}
	]
}
```

### Reconnect Bots

```
GET /reconnect
```

Response:

```json
{
	"success": true,
	"message": "Reconnect triggered for 1 bots"
}
```

### Item History

```
GET /history?uniqueId=abcd1234
```

Response:

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

```bash
GOLANG_PROTOBUF_REGISTRATION_CONFLICT=warn go run .
```

## Using the service

```bash
curl -X GET "http://localhost:3000/inspect?link=steam://rungame/730/76561202255233023/+csgo_econ_action_preview%20S76561198023809011A39999055096D14136313962912534216"
```

## Health check

```bash
curl -X GET "http://localhost:3000/health"
```

## Check item history

```bash
curl -X GET "http://localhost:3000/history?uniqueId=abcd1234"
```
