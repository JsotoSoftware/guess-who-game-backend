-- rooms.current_round_id: active round for resync and to prevent overlapping rounds.
-- Set on start_round, cleared on end_round. FK ensures we clear if round is deleted.
ALTER TABLE rooms
  ADD COLUMN current_round_id UUID NULL
  REFERENCES room_rounds(id) ON DELETE SET NULL;
