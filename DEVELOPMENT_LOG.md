# Development Log

## 2026-02-05 — Phase 1 Step 1: Security & Contract Hardening

### Completed
- Switched WebSocket auth to Bearer header parsing and removed query token support.
- Added reusable bearer token extraction helper used by both HTTP auth middleware and WS auth.
- Added configurable CORS allowlist (`CORS_ALLOW_ORIGINS`) and wired it into router startup.
- Enforced room membership checks for room members/state and room packs read/write endpoints.
- Enforced host-only permissions for selecting room packs.
- Tightened WS room join semantics to require existing room membership before joining realtime room state.

### Current status
- Backend now enforces stricter trust boundaries for room data and WS connection auth.
- This prepares the codebase for the deterministic state-machine implementation in the next phase.

### Next suggested step
- Implement server-authoritative game state machine (`LOBBY`, `MATCH_STARTING`, `ROUND_ACTIVE`, etc.) with validated transitions and turn/timer flow.
