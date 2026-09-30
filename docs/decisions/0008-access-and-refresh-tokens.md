# 0008. Short access tokens, rotating refresh tokens, per-account throttling

- **Status:** Accepted
- **Date:** 2026-10

## Context
v0.1 issued one JWT per login, valid for `JWT_EXPIRATION_HOURS`. A JWT can't be revoked, so a leaked token worked until it expired, and there was no logout. Login had no limit, so one account could be attacked with unlimited password guesses.

## Decision
- **Access token:** a JWT, short-lived (`ACCESS_TOKEN_TTL`, e.g. 15m), checked without a database lookup. It carries `role` for RBAC.
- **Refresh token:** 32 random bytes; only its SHA-256 is stored. Rotated on every use. Tokens from one login share a `family_id`; presenting an already-rotated token revokes the family (reuse detection). Logout revokes the family.
- **Rate limits (token bucket):** per IP on every route, stricter per IP on auth routes, per user on bids, and **per email on login** (5 quick attempts, then 1/min) against distributed brute force.
- `X-Forwarded-For` is only believed from `TRUSTED_PROXIES`.

## Alternatives considered
- **Server-side sessions only:** instant revocation, but a database or Redis lookup on every request.
- **Long-lived JWTs with a deny-list:** a lookup per request anyway, plus unbounded list growth.
- **Account lockout after N failures:** lets an attacker lock anyone out on purpose; throttling slows attackers without handing them that weapon.

## Consequences
- Clients must handle 401 → refresh → retry.
- A demoted admin keeps admin rights until their access token expires (≤ `ACCESS_TOKEN_TTL`).
- In-memory buckets are per instance until v0.5 moves them to Redis.
