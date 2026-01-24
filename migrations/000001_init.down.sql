-- Drop in reverse dependency order (child tables first, then parent tables)

-- Tables that depend on multiple other tables
DROP TABLE IF EXISTS round_assignments;
DROP TABLE IF EXISTS room_used_characters;

-- Must drop rooms.current_round_id before room_rounds (rooms FK references room_rounds)
ALTER TABLE rooms DROP COLUMN IF EXISTS current_round_id;

-- Tables that depend on room_rounds
DROP TABLE IF EXISTS room_rounds;

-- Tables that depend on rooms
DROP TABLE IF EXISTS room_members;
DROP TABLE IF EXISTS room_pack_selection;

-- Tables that depend on characters
DROP TABLE IF EXISTS character_translations;

-- Tables that depend on packs
DROP TABLE IF EXISTS collection_packs;
DROP TABLE IF EXISTS collection_translations;
DROP TABLE IF EXISTS collections;
DROP TABLE IF EXISTS characters;
DROP TABLE IF EXISTS pack_translations;

-- Base tables with no dependencies
DROP TABLE IF EXISTS packs;

-- Tables that depend on users (must be dropped before users)
DROP TABLE IF EXISTS refresh_sessions;
DROP TABLE IF EXISTS user_identities;
DROP TABLE IF EXISTS rooms;

-- Base user table (dropped last)
DROP TABLE IF EXISTS users;

-- Optional: keep pgcrypto extension; if you want to remove it:
-- DROP EXTENSION IF EXISTS pgcrypto;
