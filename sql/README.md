# Database SQL Scripts

This directory contains SQL scripts for setting up and maintaining the database for the CS2 Inspect Service.

## Files

- `init.sql` - Initial database setup script that creates the database, tables, and indexes

## Usage

To initialize the database using the script:

```bash
psql -U postgres -f init.sql
```

Or if you need to specify a host:

```bash
psql -h localhost -U postgres -f init.sql
```

## Database Schema

The CS2 Inspect Service uses two main tables and one materialized view:

### Asset Table

Stores information about CS2 items retrieved from the Game Coordinator:

- Item identifiers (asset_id, unique_id)
- Ownership information (ms - Steam ID)
- Item properties (paint_seed, paint_wear, quality, etc.)
- Stickers and keychains as JSONB data
- Timestamps for creation and updates

### History Table

Tracks changes to CS2 items over time:

- References to the asset (unique_id, asset_id)
- Previous and current ownership information
- Previous and current stickers/keychains
- Type of change (TRADE, MARKET_BUY, STICKER_APPLY, etc.)
- Timestamps for when the change was detected

### Rankings Materialized View

Provides float value rankings for CS2 items:

- Global rankings (highest and lowest float values across all items)
- Item-specific rankings (highest and lowest float values for specific item types)
- Rankings are calculated using PostgreSQL window functions (DENSE_RANK)
- Only includes items with valid float values (paint_wear > 0)

## Refreshing the Materialized View

The rankings materialized view needs to be refreshed periodically to include new items:

```sql
REFRESH MATERIALIZED VIEW rankings;
```

You can create a cron job or scheduled task to refresh the view automatically.

## Maintenance

If you need to make changes to the database schema, please:

1. Create a new migration script (e.g., `migrate_v1_to_v2.sql`)
2. Update this README to document the new script
3. Update the application code to work with the new schema
