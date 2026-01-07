-- Enable UUID generation
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- Users
CREATE TABLE IF NOT EXISTS users (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Identities (guest/oauth/passkey, etc.)
CREATE TABLE IF NOT EXISTS user_identities (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  provider TEXT NOT NULL,          
  provider_id TEXT NOT NULL,       
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(provider, provider_id)
);

CREATE INDEX IF NOT EXISTS idx_user_identities_user_id ON user_identities(user_id);

-- Rooms
CREATE TABLE IF NOT EXISTS rooms (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  code TEXT NOT NULL UNIQUE, 
  owner_user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  status TEXT NOT NULL DEFAULT 'active', 
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_activity_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_rooms_last_activity ON rooms(last_activity_at);

-- Room members (players in a room)
CREATE TABLE IF NOT EXISTS room_members (
  room_id UUID NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  display_name TEXT NOT NULL,
  role TEXT NOT NULL DEFAULT 'player', 
  score INT NOT NULL DEFAULT 0,
  joined_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (room_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_room_members_room_id ON room_members(room_id);

-- Packs (downloadable content packs)
CREATE TABLE IF NOT EXISTS packs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  slug TEXT NOT NULL UNIQUE,   
  version INT NOT NULL DEFAULT 1,
  is_public BOOLEAN NOT NULL DEFAULT true,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS pack_translations (
  pack_id UUID NOT NULL REFERENCES packs(id) ON DELETE CASCADE,
  lang TEXT NOT NULL,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (pack_id, lang)
);

-- Characters
CREATE TABLE IF NOT EXISTS characters (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  pack_id UUID NOT NULL REFERENCES packs(id) ON DELETE CASCADE,
  canonical_key TEXT NOT NULL,     -- stable internal key (e.g. "star_wars.darth_vader")
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(pack_id, canonical_key)
);

CREATE INDEX IF NOT EXISTS idx_characters_pack_id ON characters(pack_id);

CREATE TABLE IF NOT EXISTS character_translations (
  character_id UUID NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
  lang TEXT NOT NULL,
  name TEXT NOT NULL,              -- THIS is what you show to the player (Spanish, etc.)
  PRIMARY KEY (character_id, lang)
);

CREATE INDEX IF NOT EXISTS idx_character_translations_lang ON character_translations(lang);

CREATE TABLE IF NOT EXISTS room_pack_selection (
  room_id UUID NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
  pack_id UUID NOT NULL REFERENCES packs(id) ON DELETE CASCADE,
  selected_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (room_id, pack_id)
);

CREATE INDEX IF NOT EXISTS idx_room_pack_selection_room_id ON room_pack_selection(room_id);

-- Collections (meta-packs)
CREATE TABLE IF NOT EXISTS collections (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  slug TEXT NOT NULL UNIQUE,         -- e.g. "anime"
  is_public BOOLEAN NOT NULL DEFAULT true,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS collection_translations (
  collection_id UUID NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
  lang TEXT NOT NULL,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (collection_id, lang)
);

-- Which packs belong to a collection
CREATE TABLE IF NOT EXISTS collection_packs (
  collection_id UUID NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
  pack_id UUID NOT NULL REFERENCES packs(id) ON DELETE CASCADE,
  PRIMARY KEY (collection_id, pack_id)
);

CREATE INDEX IF NOT EXISTS idx_collection_packs_collection_id ON collection_packs(collection_id);
CREATE INDEX IF NOT EXISTS idx_collection_packs_pack_id ON collection_packs(pack_id);

-- Rounds (history + auditing)
CREATE TABLE IF NOT EXISTS room_rounds (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  room_id UUID NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
  started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  ended_at TIMESTAMPTZ,
  lang TEXT NOT NULL DEFAULT 'es'
);

CREATE INDEX IF NOT EXISTS idx_room_rounds_room_id ON room_rounds(room_id);

-- Assignments per round (who got what)
CREATE TABLE IF NOT EXISTS round_assignments (
  round_id UUID NOT NULL REFERENCES room_rounds(id) ON DELETE CASCADE,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  character_id UUID NOT NULL REFERENCES characters(id) ON DELETE RESTRICT,
  assigned_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (round_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_round_assignments_round_id ON round_assignments(round_id);

-- Used characters per room (prevents repeats within a room)
CREATE TABLE IF NOT EXISTS room_used_characters (
  room_id UUID NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
  character_id UUID NOT NULL REFERENCES characters(id) ON DELETE RESTRICT,
  first_used_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (room_id, character_id)
);

CREATE INDEX IF NOT EXISTS idx_room_used_characters_room_id ON room_used_characters(room_id);

CREATE TABLE IF NOT EXISTS refresh_sessions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,

  token_hash BYTEA NOT NULL UNIQUE, -- sha256(refresh_token)
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_used_at TIMESTAMPTZ,
  expires_at TIMESTAMPTZ NOT NULL,
  revoked_at TIMESTAMPTZ,

  user_agent TEXT NOT NULL DEFAULT '',
  ip TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_refresh_sessions_user_id ON refresh_sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_refresh_sessions_expires_at ON refresh_sessions(expires_at);