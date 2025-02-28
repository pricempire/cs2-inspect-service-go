-- CS2 Inspect Service Database Initialization
-- This script creates the necessary database and tables for the CS2 Inspect Service

-- Create database
CREATE DATABASE cs2_inspect;

-- Connect to the database
\c cs2_inspect;

-- Create asset table
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

-- Create indexes for asset table
CREATE INDEX asset_unique_id ON asset (unique_id);
CREATE INDEX asset_paint_details ON asset (paint_seed, paint_index, paint_wear);
CREATE INDEX asset_item_details ON asset (def_index, quality, rarity, origin);

-- Create history table
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

-- Create indexes for history table
CREATE INDEX history_unique_id ON history (unique_id);
CREATE INDEX history_asset_id ON history (asset_id);
CREATE INDEX history_type ON history (type);

-- Create rankings materialized view for float rankings
CREATE MATERIALIZED VIEW rankings AS
SELECT 
    DENSE_RANK() OVER(ORDER BY paint_wear DESC) AS global_low,
    DENSE_RANK() OVER(ORDER BY paint_wear ASC) AS global_high,
    DENSE_RANK() OVER(PARTITION BY paint_index, def_index, is_stattrak, is_souvenir ORDER BY paint_wear DESC) AS low_rank,
    DENSE_RANK() OVER(PARTITION BY paint_index, def_index, is_stattrak, is_souvenir ORDER BY paint_wear ASC) AS high_rank,
    asset_id
FROM asset
WHERE paint_wear IS NOT NULL AND paint_wear > 0;

-- Create unique index on rankings for asset_id
CREATE UNIQUE INDEX rankings_asset_id ON rankings (asset_id);

-- Add comments to tables and columns for better documentation
COMMENT ON TABLE asset IS 'Stores CS2 item information retrieved from the Game Coordinator';
COMMENT ON TABLE history IS 'Tracks changes to CS2 items over time';
COMMENT ON MATERIALIZED VIEW rankings IS 'Stores float rankings for CS2 items';

-- Grant permissions (adjust as needed for your environment)
-- GRANT ALL PRIVILEGES ON DATABASE cs2_inspect TO your_user;
-- GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO your_user;
-- GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO your_user; 