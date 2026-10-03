from lib import *

CODE = "/home/user/auctionEngine/backend-course/code/ch05_http"
REPO = "/home/user/auctionEngine"


def sec_repo_map():
    s = H2("13. Everything above, inside your auctionEngine repo")
    s.append(P("Reading production code is the fastest way to turn theory into skill. Here is where each idea from this part lives in your project, "
               "so you can open the file and see a real, tested version."))
    s.append(flow_row([("metrics", "counts every\nstatus"), ("LimitInFlight", "load shed\n503"), ("Recover", "panic →\n500"), ("RequestID", "X-Request-ID\nin logs")],
                      caption="Figure 13.1a  Your global middleware chain, outermost first (internal/server/server.go).", colors_=[BLUE, AMBER, RED, TEAL], box_h=36))
    s.append(flow_row([("telemetry", "trace id"), ("AccessLog", "method path\nstatus time"), ("SecurityHeaders", "nosniff, CSP,\nHSTS"), ("CORS", "allow-list +\npre-flight")],
                      caption="Figure 13.1b  …continued. Then the router, then per-route auth, per-user rate limits and the handler.", colors_=[BLUE, TEAL, RED, RED], box_h=36))
    s.append(table([
        ["Concept in this PDF", "Where in auctionEngine", "What to notice"],
        ["Handler wrapping, middleware chain", "`internal/middleware/middleware.go` `Chain`", "First listed is outermost; ordering is a design decision (metrics outermost so it counts the 500s Recover writes)"],
        ["Method + path routing", "`internal/server/server.go` `routes`, `Routes`", "`mux.Handle(r.Method+\" \"+r.Pattern, …)` uses Go 1.22 patterns, so wrong methods get 405 for free"],
        ["Consistent error bodies and JSON helpers", "`internal/httpx/httpx.go`", "`maxBodyBytes = 1 << 20` caps request bodies; one error shape everywhere"],
        ["Security headers, CORS and pre-flight", "`internal/middleware/middleware.go`", "Allow-list, `Vary: Origin`, `Max-Age`, 403 for unknown pre-flights, a CSRF explanation in the comment"],
        ["Server timeouts", "`internal/server/server.go` `New`", "ReadHeader 5 s, Read 10 s, Write 15 s, Idle 60 s"],
        ["Idempotency key on a real POST", "`internal/handlers/bid.go`, `internal/bidding/bid.go`", "Bid placement accepts `Idempotency-Key`; a reused key with a different body is `ErrIdempotencyMismatch`"],
        ["429 and Retry-After", "`internal/ratelimit/middleware.go`", "Per-IP for auth, per-user for bids"],
        ["503 and Retry-After (load shedding)", "`internal/middleware/shed.go`", "Explains congestion collapse and why rejecting early protects everyone"],
        ["Liveness vs readiness, draining", "`internal/health/health.go`, `cmd/api/main.go`", "`/readyz` answers 503 while draining so the load balancer stops sending traffic"],
        ["Graceful shutdown", "`cmd/api/main.go` (`signal.Notify`, `shutdown`)", "Stop accepting → drain → wait for workers, all under one 10 s deadline"],
        ["Long-lived connections", "`internal/realtime/hub.go`", "WebSocket connections are hijacked from net/http, so `RegisterOnShutdown(hub.Close)` is needed"],
        ["Documented decisions", "`docs/decisions/0001…0015`", "ADRs: the why behind outbox, Argon2id, SKIP LOCKED, Kafka bids, gRPC, resilience"],
    ], [2.6, 2.7, 4.2]))
    s.append(callout("interview", "In interviews, **pointing at your own project** (\"in my auction engine the bid endpoint takes an Idempotency-Key and rejects a reused key with a "
                                  "different body; the load shedder returns 503 with Retry-After instead of queueing\") is far stronger than reciting definitions."))
    return s


def sec_layout():
    s = H2("14. Code repository design for this course")
    s.append(P("The sample code is a Go module that grows chapter by chapter. Every folder is runnable on its own and tested. This mirrors how you will "
               "structure real projects: one `go.mod`, one folder per executable under `cmd/` (here, per chapter), shared logic in packages."))
    s.append(code("""backend-course/
├── code/                      # one Go module: backendcourse   (go 1.22+, standard library only)
│   ├── go.mod
│   └── ch05_http/
│       ├── rawtcp/main.go          # HTTP by hand over TCP: no magic
│       ├── server/
│       │   ├── main.go             # http.Server, timeouts, graceful shutdown
│       │   ├── handlers.go         # CRUD, ETag/304, negotiation, multipart, SSE, redirects
│       │   ├── middleware.go       # CORS, logging, gzip
│       │   ├── idempotency.go      # Idempotency-Key replay
│       │   └── server_test.go      # 10 tests: status, idempotency, CORS, cache, gzip, SSE…
│       └── client/
│           ├── client.go           # timeouts, pooling, body handling
│           └── client_test.go      # proves keep-alive reuse and context cancel
└── pdf/                       # sources that generate these PDFs (reportlab)""", lang="text", title="Layout"))
    s.append(P("**How the pieces fit** (the same shape as your auctionEngine):"))
    s.append(table([
        ["Layer", "Responsibility", "In the sample", "In auctionEngine"],
        ["`main`", "Wire config, build the server, handle signals", "`server/main.go`", "`cmd/api/main.go`"],
        ["Middleware", "Cross-cutting: log, CORS, auth, limits, panic recovery", "`middleware.go`", "`internal/middleware`"],
        ["Handler", "Parse + validate input, call service, write response", "`handlers.go`", "`internal/handlers`"],
        ["Service", "Business rules (Part 2)", "(added in Part 2)", "`internal/auction`, `internal/bidding`"],
        ["Repository", "Talks to the database (Part 5)", "(added in Part 5)", "`internal/database`"],
    ], [1.2, 3.4, 2, 2.6]))
    s.append(Paragraph("Run it yourself", S["h3"]))
    s.append(code("""cd backend-course/code
go test -race ./...                 # 13 tests, race detector on
go run ./ch05_http/server           # http://localhost:8082
go run ./ch05_http/rawtcp           # http://localhost:8081   (hand-written HTTP)

# poke it
curl -i  localhost:8082/hello -H 'Accept-Language: es'
curl -i  -X POST localhost:8082/notes -H 'Idempotency-Key: k1' -d '{"text":"hi"}'
curl -i  localhost:8082/notes/1
curl -i  -H 'If-None-Match: "<etag from above>"' localhost:8082/notes/1      # 304
curl -i  -X OPTIONS localhost:8082/notes/1 -H 'Origin: http://localhost:5173' \\
         -H 'Access-Control-Request-Method: PUT'                              # pre-flight
curl -sN localhost:8082/stream                                                 # SSE
curl -s  -H 'Accept-Encoding: gzip' localhost:8082/big | wc -c               # compressed size""", lang="bash", title="Commands"))
    return s


def sec_gaps():
    s = H2("15. Beyond the video: what else interviews ask about HTTP")
    s.append(P("The video is an excellent base. These are the HTTP topics it does not cover (or only names) that come up in interviews and real incidents."))
    s.append(table([
        ["Topic", "What you must be able to say"],
        ["Cookies", "`Set-Cookie` / `Cookie`; attributes `HttpOnly`, `Secure`, `SameSite`, `Domain`, `Path`, `Max-Age`; cookies are sent automatically by the browser, which is exactly what makes **CSRF** possible. Header-based tokens are not sent automatically."],
        ["Redirect loops and 307/308", "Method preservation; `Location` header; caching of 301; open-redirect vulnerability when you redirect to a user-supplied URL."],
        ["Reverse proxy / forward proxy / load balancer", "Reverse proxy sits in front of servers (TLS, caching, routing); forward proxy sits in front of clients. L4 vs L7 balancing. Real client IP comes from `X-Forwarded-For`; trust it only from known proxies."],
        ["DNS", "Domain → IP lookup; TTL and caching; A/AAAA/CNAME; why a deploy \"works for some users\" (stale DNS)."],
        ["Timeouts and Slowloris", "`ReadHeaderTimeout` defeats clients that dribble headers byte by byte to hold connections open."],
        ["Head-of-line blocking", "HTTP/1.1 at the HTTP level, HTTP/2 at the TCP level, solved in HTTP/3 (QUIC)."],
        ["Trailers and 100-continue", "`Expect: 100-continue` lets a client ask permission before sending a large body; rarely needed but appears in interviews."],
        ["Retry-After and back-off", "429/503 should include `Retry-After`; clients retry with **exponential back-off and jitter**, only for idempotent operations."],
        ["Request size and abuse limits", "Cap body, header size and URL length; reject early. `MaxBytesReader`, `MaxHeaderBytes`."],
        ["Content-Disposition and downloads", "`attachment; filename=…` for downloads; sanitise filenames; set a correct `Content-Type`."],
        ["HTTP/2 and gRPC", "gRPC needs HTTP/2 because it relies on multiplexed streams and trailers (Part 9)."],
        ["Observability headers", "`X-Request-ID` / `traceparent` propagate a trace id across services; return it in responses so support can find the exact request."],
    ], [2.2, 7.3]))
    return s


def sec_qa():
    s = H1("16. Interview preparation", "PART 1 · FACT QUESTIONS, THEN SITUATIONS")
    s.append(Paragraph("A. Core questions (the video's, refined)", S["h3"]))
    qa = [
        ("1. What does \"HTTP is stateless\" mean, and how do you keep a user logged in?",
         "The server keeps no memory between requests; each request must be self-contained. Login continuity is simulated by sending a session cookie or a bearer token with every request, "
         "and the server looks up or verifies it each time. That statelessness is what lets any replica serve any request."),
        ("2. PUT vs PATCH?",
         "PUT replaces the whole resource with the body (idempotent). PATCH applies a partial change (not guaranteed idempotent). Use PATCH for typical updates; PUT when the client owns the id or wants a full replace."),
        ("3. What is idempotency and which methods are idempotent?",
         "Repeating the request leaves the server in the same state as sending it once. GET, HEAD, PUT, DELETE and OPTIONS are; POST is not; PATCH depends on the operation. "
         "It is about state, not responses: a second DELETE may return 404."),
        ("4. Your React app on :3000 calls your API on :3001 and the browser blocks it. Why, and how do you fix it?",
         "Different ports mean different origins, so the same-origin policy hides the response unless the API opts in. Fix it on the **server**: return `Access-Control-Allow-Origin: http://localhost:3000` "
         "(an allow-list, not `*`), and answer the OPTIONS pre-flight with 204 plus `Allow-Methods`, `Allow-Headers` and `Max-Age`. Alternatively proxy `/api` through the dev server so it is same-origin."),
        ("5. When does the browser send a pre-flight request?",
         "Only for cross-origin requests that are not \"simple\": method other than GET/HEAD/POST, or a non-safelisted header (`Authorization`, custom headers), or a `Content-Type` other than form-urlencoded, multipart or text/plain (so JSON always pre-flights)."),
        ("6. Explain HTTP caching with ETag and 304.",
         "The server returns a validator (`ETag`) and freshness info (`Cache-Control`). When the copy is stale the client sends `If-None-Match`. If it still matches, the server replies 304 with no body and the client reuses its copy; otherwise 200 with a new ETag."),
        ("7. You get 502 Bad Gateway. What does it mean?",
         "A proxy or load balancer received an invalid response (or none: connection refused) from the upstream app. Check that the app is running, listening on the expected port, and not crashing. 504 means the upstream was too slow."),
        ("8. What is content negotiation?",
         "The client states preferred format, language and encoding with `Accept`, `Accept-Language`, `Accept-Encoding`; the server picks the best match, reports it with `Content-Type`, `Content-Language`, `Content-Encoding`, and sets `Vary`. If nothing matches: 406."),
        ("9. How would you let users upload a 2 GB video?",
         "Do not send it as JSON and do not buffer it in memory. Use multipart streaming with a hard size limit, or better, issue a **pre-signed URL** so the client uploads directly to object storage, with resumable or chunked upload for reliability; "
         "process it asynchronously with a queue."),
        ("10. How do you push live logs from the server to the browser?",
         "Server-Sent Events: `Content-Type: text/event-stream`, flush each event, stop on `r.Context().Done()`. If the client must also send messages, use WebSocket. Mind proxy buffering and server write timeouts."),
    ]
    for q, a in qa:
        s += QA(q, a)
    s.append(Paragraph("B. Deeper protocol questions", S["h3"]))
    qb = [
        ("11. Why does HTTP/2 not fully fix head-of-line blocking, and what does HTTP/3 change?",
         "HTTP/2 multiplexes streams over **one TCP connection**; TCP delivers bytes in order, so one lost packet stalls every stream. HTTP/3 uses QUIC over UDP with independent streams, so only the affected stream waits; it also merges transport and TLS handshakes."),
        ("12. 401 or 403 for \"valid token but not an admin\"? And for \"no token\"?",
         "No/invalid token: 401 (unauthenticated, with `WWW-Authenticate`). Valid token, insufficient role: 403. For resources the user must not even know about, 404."),
        ("13. Difference between `no-cache` and `no-store`?",
         "`no-cache` allows storing but forces revalidation before reuse. `no-store` forbids storing at all. Use `no-store` for sensitive data."),
        ("14. What is the difference between a reverse proxy and a forward proxy?",
         "A forward proxy acts for clients (corporate proxy, VPN). A reverse proxy acts for servers (Nginx, load balancer, CDN): terminates TLS, balances load, caches, hides backends."),
        ("15. Is CORS a security feature that protects my API?",
         "No. It is a browser rule that protects **users** from other sites reading their data. Attackers' own scripts, curl and bots ignore it. Protect the API with authentication, authorisation, and rate limits."),
        ("16. What is `Vary` and why does it matter?",
         "It lists request headers that influenced the response (`Origin`, `Accept-Encoding`, `Accept-Language`). Caches must include them in the cache key; forgetting it lets a CDN serve a gzip body to a client that cannot decode it, or one origin's CORS header to another."),
        ("17. Why must CORS middleware run before authentication?",
         "Pre-flight OPTIONS requests carry no credentials. If auth runs first it returns 401 and the browser reports a CORS failure, so the real request is never sent."),
        ("18. What goes wrong with `http.Get` in production?",
         "No timeout (hung goroutines, cascading failure), possible connection leaks if the body is not closed, and pool starvation with the default 2 idle connections per host. Use a shared client with timeouts, contexts and tuned transport."),
        ("19. What does `ReadHeaderTimeout` protect against?",
         "Slowloris: a client opens many connections and trickles header bytes to exhaust the server's connections. A short header timeout closes them."),
        ("20. TLS handshake: what is it for, and what does the client verify?",
         "To agree on keys and authenticate the server. The client checks the certificate chain leads to a trusted CA, the hostname matches, and the certificate is within its validity period."),
    ]
    for q, a in qb:
        s += QA(q, a)
    s.append(Paragraph("C. Situation and scenario questions", S["h3"]))
    qc = [
        ("21. The user clicks \"Place bid\" twice, or the mobile app retries after a timeout. How do you avoid two bids?",
         "Make the operation idempotent. The client generates an `Idempotency-Key` once per user intent and reuses it on retries; the server stores key → result (Redis/DB, with TTL, bound to the user) and replays the stored response. "
         "Same key with a different body is rejected (422/409). Handle two identical requests arriving simultaneously with a lock or unique constraint. Your auction engine does this in the bid path."),
        ("22. After a frontend deploy some users see the old UI, others the new one. Why?",
         "Caching: the browser or CDN holds an old `index.html` or old JS. Serve HTML with `no-cache` (revalidate), serve JS/CSS with content-hashed names and `max-age=31536000, immutable`, and purge the CDN entry point if needed."),
        ("23. Every API call from the browser fails with a CORS error, but curl works. Walk me through the debugging.",
         "1) Open the Network tab: is there a failed OPTIONS? 2) Compare `Origin` with the server allow-list (scheme, host, port, trailing slash). 3) Check the pre-flight reply: status 2xx, "
         "`Allow-Methods`/`Allow-Headers` contain what was requested. 4) Check error responses also carry CORS headers. 5) Check auth middleware is not rejecting OPTIONS. 6) With cookies: no `*`, `Allow-Credentials: true`, and `SameSite` settings."),
        ("24. Latency doubled but CPU is idle. Where do you look?",
         "Connection handling and dependencies, not compute: missing keep-alive or tiny client pool (new TCP/TLS per call), DNS lookups, a slow downstream with no timeout, DB pool exhaustion, a proxy buffering, or a TLS handshake storm after a deploy. Check traces and per-phase timings (DNS, connect, TLS, first byte)."),
        ("25. Your service returns 502 for about 30 seconds on every deploy. Why and how do you fix it?",
         "Instances are killed while still receiving traffic or the new one is not ready. Fix: a readiness endpoint that fails while draining, a delay before closing the listener, graceful `Shutdown` that finishes in-flight requests, "
         "and rolling deploys that wait for readiness. This is exactly what `cmd/api/main.go` implements."),
        ("26. A partner needs to download a 5 GB report. How do you serve it?",
         "Generate it asynchronously (202 Accepted + job id), store it in object storage, and return a time-limited pre-signed URL. If you must stream from the app: chunked response with flushing, `Content-Disposition`, "
         "support `Range` for resume, no server `WriteTimeout` on that route, and cancel work on client disconnect."),
        ("27. You must show live auction prices to 50,000 viewers. HTTP polling, SSE or WebSocket?",
         "Viewers only receive, so SSE or WebSocket both fit; polling every second means 50,000 requests per second of pure overhead. Choose WebSocket if you need client messages or already run it; SSE for simplicity. "
         "Either needs a fan-out layer (pub/sub through Redis/Kafka) so any instance can push, connection limits and heartbeats, and shedding. See Part 7."),
        ("28. An endpoint sometimes returns 200 with an error message in the body. What is wrong and why does it matter?",
         "It breaks the contract: caches may store it, monitors count it as success, clients' retry logic and generated SDKs mis-handle it. Use the right 4xx/5xx and a consistent error body."),
        ("29. How would you rate limit and what do you return?",
         "Identify the caller (user id after auth, otherwise IP via a trusted proxy header), apply a token-bucket or sliding window (shared in Redis when multiple replicas), return 429 with `Retry-After` and rate-limit headers. Separate limits for login, bids and reads. (Part 10.)"),
        ("30. Design the HTTP behaviour of a payment endpoint.",
         "POST with `Idempotency-Key`, TLS only, strict input validation, 201 with `Location` or 202 if async, `Cache-Control: no-store`, 409/422 on key reuse mismatch, never log card data, timeouts at every hop, "
         "webhooks to confirm final state with signature verification, and a reconciliation job. Retries by the client are safe only because of the key."),
    ]
    for q, a in qc:
        s += QA(q, a)
    return s


def sec_cheat():
    s = H2("17. One-page cheat sheet")
    s.append(table([
        ["I need to…", "Do this"],
        ["Create a resource", "`POST /things` → **201** + `Location: /things/42`"],
        ["Replace / partially update", "`PUT /things/42` (full) · `PATCH /things/42` (partial) → **200** or 204"],
        ["Delete", "`DELETE /things/42` → **204** (repeat → 404 is fine)"],
        ["Make POST retry-safe", "`Idempotency-Key` header + stored response"],
        ["Bad input", "**400** (or 422) + `{\"error\":…,\"fields\":{…}}`"],
        ["Not logged in / not allowed", "**401** (+ `WWW-Authenticate`) / **403** (or 404 to hide)"],
        ["Duplicate or edit collision", "**409** (or **412** with `If-Match`)"],
        ["Too many requests", "**429** + `Retry-After`"],
        ["Overloaded or draining", "**503** + `Retry-After`"],
        ["Cache a public read", "`Cache-Control: public, max-age=N` + `ETag`; honour `If-None-Match` → **304**"],
        ["Never cache", "`Cache-Control: no-store`"],
        ["Support a browser on another origin", "CORS allow-list middleware, `Vary: Origin`, answer OPTIONS with 204, run before auth"],
        ["Serve many formats / languages", "Read `Accept*`, set `Content-*` and `Vary`, 406 if none"],
        ["Big download / upload", "Stream; pre-signed URLs; `MaxBytesReader`; `Range` support"],
        ["Live updates server → client", "SSE (`text/event-stream`, flush, `r.Context().Done()`) or WebSocket"],
        ["Outgoing HTTP call", "Shared `http.Client` with timeouts, context, `defer resp.Body.Close()`, check status"],
        ["Server hardening", "`ReadHeaderTimeout`, `ReadTimeout`, `IdleTimeout`, body size cap, graceful `Shutdown`, security headers"],
    ], [3, 6.5]))
    s.append(Paragraph("Exercises (do these before Part 2)", S["h3"]))
    s += numbered([
        "Run the raw TCP server and send a request with `nc`. Add a `POST /echo` route that returns the request body (you must read `Content-Length` bytes).",
        "Add `If-Match` support to `PUT /notes/{id}`: return **412** when the ETag does not match (optimistic locking, the fix for lost updates). Write the test first.",
        "Add `Retry-After` and a **429** to the sample server with a small token bucket per client IP.",
        "Make `GET /notes` return a list with `Link`-based or cursor pagination and a weak ETag for the list.",
        "Serve a file with `http.ServeContent` and verify `Range: bytes=0-99` returns **206** with `Content-Range`.",
        "In the browser, from a page on `http://localhost:5173`, call `PUT http://localhost:8082/notes/1` with JSON and watch the OPTIONS pre-flight in the Network tab; then remove one CORS header and read the error.",
        "Read `internal/middleware/shed.go` in auctionEngine and write, in your own words, why returning 503 immediately is better than queueing.",
    ])
    s.append(callout("tip", "**Coming next, Part 2:** routing, serialization/deserialization (JSON, protobuf, encoding pitfalls), validation and transformation, and the controller → service → repository "
                            "layering with `context`, using Go code you can run. Paste the transcripts of the next videos and I will check the PDF against them."))
    return s
