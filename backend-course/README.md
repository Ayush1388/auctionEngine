# Backend from First Principles: Go edition

Study PDFs plus runnable, tested Go code. Built from the Sriniously "Backend from first principles"
course, the GitHub syllabus (DsThakurRawat/Backend-from-first-Principle), RFCs, and this repo's own
auctionEngine code.

| Part | Topic | PDF | Code |
|------|-------|-----|------|
| 1 | HTTP: methods, headers, CORS, status codes, caching, negotiation, streaming, TLS | `pdf/Part1-HTTP.pdf` | `code/ch05_http/` |

## Run the code

```sh
cd code
go test -race ./...            # 13 tests
go run ./ch05_http/server      # http://localhost:8082
go run ./ch05_http/rawtcp      # http://localhost:8081  (HTTP spoken by hand over TCP)
```

Standard library only; needs Go 1.22+ (method + wildcard routing in `net/http`).

## Rebuild the PDF

```sh
pip install reportlab pygments
cd pdf && python3 build_part1.py
```

Code excerpts in the PDF are pulled from the real source files at build time, so they cannot drift from
the tested code.
