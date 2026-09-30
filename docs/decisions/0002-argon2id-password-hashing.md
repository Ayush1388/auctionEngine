# 0002. Argon2id for password hashing

- **Status:** Accepted
- **Date:** 2026-09

## Context
Passwords must be stored so that a leaked database doesn't hand attackers the plaintext. The hash has to be slow and expensive to compute, especially on GPUs.

## Decision
Use **Argon2id** (`golang.org/x/crypto/argon2`) with a random 16-byte salt per password: time = 1, memory = 64 MiB, threads = 4, key length = 32. The hash is stored in the standard PHC format (`$argon2id$v=19$m=...,t=...,p=...$salt$hash`), so the parameters travel with it.

## Alternatives considered
- **bcrypt** – solid and widely used, but not memory-hard, and it silently truncates passwords at 72 bytes.
- **SHA-256 with a salt** – far too fast; billions of guesses per second on a GPU.

## Consequences
- Each login costs about 64 MiB of memory for a short time, so the login endpoint needs rate limiting before production.
- Verification uses `subtle.ConstantTimeCompare` to avoid timing leaks.
- `CheckPassword` currently verifies with the constants in code rather than the parameters parsed from the stored hash. Raising the cost later would break existing logins until verification reads the stored parameters.
