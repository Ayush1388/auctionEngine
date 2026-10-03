from lib import *

CODE = "/home/user/auctionEngine/backend-course/code/ch05_http"
REPO = "/home/user/auctionEngine"


def cover():
    s = []
    s.append(Spacer(1, 38 * mm))
    s.append(Paragraph("BACKEND FROM FIRST PRINCIPLES  ·  GO EDITION", S["kicker"]))
    s.append(Paragraph("Part 1<br/>HTTP: where<br/>everything starts", ParagraphStyle(
        "cv", fontName="DVB", fontSize=40, leading=44, textColor=INK, spaceAfter=14)))
    s.append(Paragraph(fmt(
        "What it is, why it exists, when to use it, when not to, the trade-offs, and how to build it "
        "in Go. Every code sample in this PDF compiles and is covered by tests."),
        ParagraphStyle("cv2", parent=S["body"], fontSize=11.5, leading=17, textColor=MUTED, spaceAfter=18)))
    s.append(Spacer(1, 6 * mm))
    rows = [
        ["Covers", "Client-server model · statelessness · TCP/TLS/HTTP versions · message anatomy · headers · methods and idempotency · CORS and pre-flight · status codes · caching · content negotiation · compression · keep-alive · multipart and streaming · HTTPS"],
        ["Sources", "Sriniously video 5 (\"Understanding HTTP for backend engineers\") transcript · GitHub repo DsThakurRawat/Backend-from-first-Principle (26-topic syllabus) · your own auctionEngine Go repo · gaps filled from RFC 9110/9111/9112 and the Fetch spec"],
        ["Runnable code", "`backend-course/code/ch05_http/` (raw TCP server, full net/http server, HTTP client; 13 tests, race-detector clean)"],
        ["How to read", "Each topic has a study card (what / why / when / when not / pros / cons / how), diagrams, Go code, a pointer to the matching file in auctionEngine, and interview angles. Part 1 ends with 30 interview questions including scenario questions."],
    ]
    s.append(table(rows, [1.1, 6], header=False, zebra=False, bold_first=True))
    s.append(Spacer(1, 10 * mm))
    s.append(callout("tip", [
        "**Honest scope note.** The transcript I had was for video 5 only, and the Vercel site and YouTube were blocked in this "
        "environment. The rest of the syllabus comes from the GitHub repo's folder names, so later parts will be checked against "
        "those folders (and your transcripts when you paste them). Points where I correct or refine something the video said are "
        "marked **CORRECTION / NUANCE**."]))
    s.append(PageBreak())
    return s


def roadmap():
    s = H1("The complete roadmap", "00 · HOW THE SERIES IS ORGANISED")
    s.append(P("The GitHub repo lists 26 topics. Below is each one mapped to a PDF in this series, plus a second list of topics "
               "that interviews ask about but the course does not cover in depth. Nothing here is skipped: if a topic is "
               "not in the course, it is in the gap list."))
    rows = [["#", "Course topic (from the repo)", "Part", "Why it matters"],
            ["1", "HTTP and CORS  ·  **this PDF**", "1", "Every other topic rides on HTTP"],
            ["2", "Routing in the backend", "2", "Mapping method + path to code"],
            ["3", "Serialization and deserialization", "2", "JSON, validation of untrusted input, protobuf"],
            ["4", "Authentication and authorization", "3", "Sessions, JWT, OAuth2, RBAC, password hashing"],
            ["5", "Validations and transformations", "2", "Never trust the client"],
            ["6", "Controllers, services, repositories, middlewares, request context", "2", "Layering: where code lives"],
            ["7", "API design (REST)", "4", "Resources, pagination, versioning, idempotency"],
            ["8", "Databases with the backend", "5", "SQL, indexes, transactions, isolation, pooling, migrations"],
            ["9", "Caching", "6", "Redis, cache-aside, stampede, invalidation"],
            ["10", "Task queues and background jobs", "7", "Retries, outbox, SKIP LOCKED workers"],
            ["11", "Full-text search (Elasticsearch)", "6", "Inverted index, search as a read model"],
            ["12", "Error handling and fault-tolerant systems", "8", "Timeouts, retries, circuit breakers, bulkheads"],
            ["13", "gRPC and inter-service communication", "9", "Protobuf, streaming, REST vs gRPC"],
            ["14", "Production configuration management", "8", "12-factor, secrets, env vs files"],
            ["15", "Logging, monitoring, observability", "8", "Logs, metrics, traces, SLOs"],
            ["16", "Graceful shutdown", "8", "Draining, SIGTERM, in-flight work"],
            ["17", "Backend security", "3", "OWASP top 10, CSRF, XSS, SQLi, SSRF, headers"],
            ["18", "Scaling and performance, part 1", "10", "Load balancing, horizontal vs vertical, statelessness"],
            ["19", "Scaling and performance, part 2", "10", "Sharding, replication, CDN, back-pressure"],
            ["20", "Concurrency and parallelism, IO vs CPU bound", "11", "Goroutines, channels, context, worker pools"],
            ["21", "Containerization and deployment (Docker, K8s, CI/CD)", "12", "Shipping what you build"],
            ["22", "Automated testing (unit, integration, e2e)", "12", "Table tests, httptest, testcontainers"],
            ["23", "Message brokers and event streaming (Kafka)", "9", "Ordering, consumer groups, delivery semantics"],
            ["24", "WebSockets and real-time communication", "7", "Fan-out, heartbeats, scaling stateful connections"],
            ["25", "OpenAPI specification", "4", "Contract-first, generated docs and clients"],
            ["26", "AI agents and loop engineering", "13", "Tool calling, loops, guardrails"]]
    s.append(table(rows, [0.5, 4.7, 0.8, 5]))
    s.append(Paragraph("Gap list: what interviews ask that the course does not teach in depth", S["h3"]))
    gaps = [
        ["Networking", "DNS resolution, CDN, forward vs reverse proxy, NAT, WebSocket upgrade handshake, HTTP/2 streams", "1, 10"],
        ["Go language", "Goroutine scheduler, channels vs mutexes, `context`, memory model, escape analysis, `pprof`, generics, error wrapping", "11"],
        ["Databases deep-dive", "B-tree vs LSM, query plans, N+1, deadlocks, isolation anomalies, optimistic vs pessimistic locking, SQL vs NoSQL", "5"],
        ["Distributed systems", "CAP and PACELC, consistency models, consensus (Raft idea), idempotency, saga, outbox, exactly-once myths", "9, 10"],
        ["System design", "URL shortener, rate limiter, chat, news feed, notification system, payment, **auction/bidding** (your project)", "14"],
        ["Rate limiting and abuse", "Token bucket, leaky bucket, sliding window, per-user vs per-IP, distributed limiters", "10"],
        ["Security extras", "JWT pitfalls, refresh rotation, password hashing (Argon2id), SSRF, mass assignment, secrets management", "3"],
        ["Operations", "Blue/green and canary, feature flags, zero-downtime migrations, backups, incident response, SLO and error budgets", "12"],
        ["Behavioural/situational", "\"Production is slow\", \"500s spiking\", \"DB at 100% CPU\", \"traffic 10x\", debugging playbooks", "14"],
    ]
    s.append(table([["Area", "Topics I will add", "Part"]] + gaps, [1.95, 6.6, 0.8]))
    s.append(callout("tip", "The part numbers are a plan, not a promise: ask for a different order, or for any topic to be split or "
                            "merged, and I will adjust. Part 14 (system design and incident scenarios) comes last on purpose: it "
                            "reuses everything before it."))
    s.append(PageBreak())
    return s


def sec_big_picture():
    s = H1("1. The big picture: clients, servers and a request's journey", "01 · START HERE")
    s.append(P("A **backend** is a program that waits for requests from other programs, does some work (check permissions, read or write "
               "a database, call another service) and sends back a response. A **client** is whoever starts the conversation: a browser, "
               "a mobile app, `curl`, or another server. A **server** is whoever waits and answers. In HTTP the client always speaks "
               "first; the server never sends anything the client did not ask for (the exceptions, SSE and WebSockets, are all "
               "started by a client request too)."))
    s.append(flow_row([("Client", "browser / app\n/ curl"), ("Internet", "DNS + TCP\n+ TLS"), ("Reverse proxy", "TLS ends here\nNginx / LB"),
                       ("Go server", "net/http\nmiddleware"), ("Handler", "validate +\nroute"), ("Service", "business\nrules"), ("Database", "Postgres\n/ Redis")],
                      caption="Figure 1.1  A request's journey. Each box is a place where latency, errors and security decisions live.",
                      colors_=[INK, MUTED, TEAL, RED, RED, RED, BLUE], box_h=40, sizes=7.2))
    s += card("Client-server model",
              "Two roles. The client initiates, the server listens and responds. They agree on a protocol (HTTP) for the conversation.",
              "Separation of concerns: the UI can change without touching the data layer, many different clients (web, iOS, partners) can share one backend, and servers can be scaled and secured independently.",
              "Almost every networked application: web apps, mobile apps, public APIs, internal microservices.",
              "When both sides need to push data freely (chat, games) pure request/response is awkward: use WebSockets, SSE or a message broker on top of it. Peer-to-peer apps have no central server.",
              "Central place for rules and data (one source of truth); easy to monitor and secure; clients stay thin.",
              "The server is a single point of failure and a bottleneck unless you scale it; every interaction pays network latency.",
              "`net.Listen` + `Accept`, or simply `http.ListenAndServe`. Later parts add layering, config and graceful shutdown.")
    s.append(P("**Mental model to keep for the whole series:** a backend is a pipeline. Bytes arrive, get parsed into a request, pass "
               "through a chain of cross-cutting steps (logging, auth, CORS, rate limits), reach a handler that validates input and calls "
               "business logic, which talks to storage, and the answer travels back out. Every later chapter is one stage of this pipeline."))
    return s


def sec_stateless():
    s = H2("2. Statelessness: the first core idea")
    s.append(P("HTTP has **no memory between requests**. Request 2 does not know request 1 happened. Everything the server needs "
               "(who you are, what you want, the format you accept) must travel inside each request, in the URL, headers, "
               "cookies or body."))
    s.append(sequence(["Client", "Server"], [
        (0, 1, "GET /profile  Authorization: Bearer abc"), (1, 0, "200 OK  {name: Asha}"),
        (1, 1, "server forgets the request", "note"),
        (0, 1, "GET /profile   (no token)"), (1, 0, "401 Unauthorized  (it does not remember you)")],
        caption="Figure 2.1  Two independent requests. The second fails because the client did not resend its credentials.",
        lane_colors=[INK, RED]))
    s += card("Stateless protocol (and how we fake state)",
              "The server stores nothing about a client between requests. Continuity is simulated by the client re-sending a token or a session id each time.",
              "Any server instance can answer any request, so you can add or remove servers freely, and a crash loses no conversation.",
              "Always for the HTTP layer. Prefer stateless designs for APIs that must scale horizontally.",
              "Do not push everything into the client: large per-user state in a JWT bloats every request. Truly stateful needs (WebSocket connections, in-progress uploads, game rooms) need sticky routing or an external store.",
              "Simple servers; trivial horizontal scaling and load balancing; failures are isolated; easy caching.",
              "Credentials/context are resent every time (bigger requests); revoking a self-contained token is hard; some features (shopping carts) need an external store anyway.",
              "Keep state **outside the process**: in Postgres or Redis, or inside a signed token. A Go handler must not rely on a package-level map surviving between requests when you run two replicas.")
    s.append(table([
        ["Technique", "Where the state lives", "Good for", "Watch out for"],
        ["Server session + cookie", "Server store (Redis/DB); browser holds only a random id", "Browser apps; instant revocation", "Needs shared store when scaled; CSRF protection needed"],
        ["Signed token (JWT)", "Inside the token the client sends", "APIs, mobile, microservices", "Cannot be revoked before expiry without extra work; keep payload small"],
        ["Opaque access token + DB lookup", "Server store; client holds random token", "Revocable API auth", "One lookup per request (cache it)"],
    ], [1.5, 2.2, 1.7, 2.6]))
    s.append(callout("repo", "auctionEngine keeps both: short-lived access tokens plus refresh tokens (see `docs/decisions/0008-access-and-refresh-tokens.md`) "
                             "and a `internal/session` package. Each request carries its own `Authorization` header, which is exactly the "
                             "stateless pattern. Redis is treated as a disposable accelerator (ADR 0009), never the only copy of important state."))
    return s


def sec_transport():
    s = H2("3. Under HTTP: TCP, TLS and the versions of HTTP")
    s.append(P("HTTP lives at **layer 7 (application)** of the OSI model. It needs a **reliable, ordered byte stream** underneath, "
               "which is what TCP provides (HTTP/3 gets the same guarantees from QUIC over UDP). As a backend engineer you rarely write "
               "the lower layers, but you must know what they cost, because that is where latency and many production mysteries come from."))
    s.append(layers([("7  Application", "HTTP, gRPC, DNS, SMTP  ← you work here"), ("6  Presentation", "TLS encryption, encoding"),
                     ("5  Session", "connection management"), ("4  Transport", "TCP (reliable) or UDP / QUIC"),
                     ("3  Network", "IP routing"), ("2  Data link", "Ethernet, Wi-Fi"), ("1  Physical", "cables, radio")],
                    hl=[0], caption="Figure 3.1  The OSI model. HTTP is layer 7; TLS and TCP are what it stands on."))
    s.append(sequence(["Client", "Server"], [
        (0, 1, "SYN"), (1, 0, "SYN-ACK"), (0, 1, "ACK   (1 round trip: TCP connected)"),
        (0, 1, "ClientHello (TLS 1.3, key share)"), (1, 0, "ServerHello + certificate + Finished"),
        (0, 1, "Finished   (+1 round trip: encrypted)"), (0, 1, "GET /  (first HTTP request, encrypted)")],
        caption="Figure 3.2  What happens before your first byte of HTTP over HTTPS/1.1 or 2: about 2 round trips (TCP, then TLS 1.3).",
        lane_colors=[INK, RED]))
    s.append(table([
        ["", "TCP", "UDP"],
        ["Delivery", "Reliable, ordered, retransmits lost packets", "Best effort: may lose, duplicate or reorder"],
        ["Setup", "3-way handshake (1 round trip)", "None"],
        ["Use", "HTTP/1.1, HTTP/2, databases, SSH", "DNS, video calls, games, QUIC (HTTP/3)"],
        ["Cost", "Handshake latency, head-of-line blocking at the transport layer", "You (or QUIC) must handle reliability"],
    ], [0.8, 3, 3]))
    s.append(P("**The four HTTP versions, and the problem each one solved:**"))
    s.append(table([
        ["Version", "Key idea", "Problem it fixed", "Remaining weakness"],
        ["HTTP/1.0 (1996)", "One request per TCP connection", "First usable version", "Open and close a connection for every file: slow"],
        ["HTTP/1.1 (1997)", "Persistent connections (keep-alive), chunked transfer, `Host` header, better caching", "Connection setup cost; virtual hosting", "Responses on one connection are in order, so a slow one blocks the next (application-level head-of-line blocking). Browsers open about 6 connections per host to cope"],
        ["HTTP/2 (2015)", "**Binary framing**, **multiplexing** many streams on one connection, HPACK header compression", "Head-of-line blocking at the HTTP level; verbose headers", "Still on TCP: one lost packet stalls **all** streams (TCP head-of-line blocking)"],
        ["HTTP/3 (2022)", "Runs on **QUIC over UDP**: per-stream loss recovery, TLS 1.3 built in, 0/1-RTT setup, QPACK", "TCP head-of-line blocking; slow handshakes; connection survives network change", "UDP may be blocked or throttled by some networks; more CPU per byte"],
    ], [1.2, 2.6, 2.1, 3]))
    s.append(callout("gotcha", [
        "The video mentions **server push** as an HTTP/2 feature. It existed in the spec but proved hard to use well: Chrome removed support "
        "in 2022 and it is effectively dead. Use `103 Early Hints` or `<link rel=preload>` instead.",
        "The video says TLS 1.2 is \"the current recommended version\" (the transcript itself flags that part as uncertain). Today **TLS 1.3** "
        "is the recommended version and 1.2 is still acceptable; TLS 1.0 and 1.1 are deprecated (RFC 8996)."]))
    s.append(callout("whennot", "You almost never choose the HTTP version in application code. Go's `net/http` negotiates HTTP/2 automatically over "
                                "TLS (via ALPN). Choose consciously only at the edge (CDN, load balancer, Nginx), and remember **gRPC requires HTTP/2**."))
    return s


def sec_message():
    s = H2("4. Anatomy of an HTTP message")
    s.append(P("HTTP/1.1 messages are plain text. A request is: **request line**, **headers**, a **blank line**, then an optional "
               "**body**. A response is: **status line**, **headers**, a **blank line**, then the **body**. (HTTP/2 and 3 send the same "
               "information in binary frames, but the concepts and header names are identical.)"))
    s.append(message_anatomy([
        ("POST /v1/auctions/42/bids HTTP/1.1", "request line: method, target, version", RED),
        ("Host: api.example.com", "headers: Key: value", AMBER),
        ("Content-Type: application/json", "", AMBER),
        ("Authorization: Bearer eyJhbGci...", "", AMBER),
        ("Content-Length: 18", "", AMBER),
        ("", "blank line = headers are over", colors.HexColor("#9AA08A")),
        ('{"amount": 25000}', "request body (optional)", GREEN)],
        caption="Figure 4.1  A request message."))
    s.append(message_anatomy([
        ("HTTP/1.1 201 Created", "status line: version, code, reason", RED),
        ("Content-Type: application/json", "headers", AMBER),
        ("Location: /v1/bids/9001", "", AMBER),
        ("Content-Length: 41", "", AMBER),
        ("", "blank line", colors.HexColor("#9AA08A")),
        ('{"id": 9001, "amount": 25000}', "response body", GREEN)],
        caption="Figure 4.2  A response message."))
    s.append(P("To prove there is no magic, here is a server that speaks HTTP/1.1 by hand over a raw TCP socket (about 70 lines). "
               "`net/http` does exactly this, plus the hundreds of edge cases (chunked bodies, timeouts, keep-alive, HTTP/2)."))
    s.append(code(grab(f"{CODE}/rawtcp/main.go", r"^func handle", r"^}"), title="backend-course/code/ch05_http/rawtcp/main.go  (the parsing part)"))
    s.append(code(grab(f"{CODE}/rawtcp/main.go", r"^func respond", r"^}"), title="…and the response writer"))
    s.append(code("""$ curl -i localhost:8081/hello -H 'User-Agent: demo'
HTTP/1.1 200 OK
Content-Type: text/plain; charset=utf-8
Content-Length: 33
Connection: close

hello from raw TCP, you are demo

$ curl -si -X POST localhost:8081/hello | head -1
HTTP/1.1 405 Method Not Allowed""", lang="text", title="Real output from running the program"))
    s.append(callout("tip", "`Content-Length` (or chunked encoding) is how the receiver knows where the body ends on a connection that stays open. "
                            "Get it wrong and the client hangs waiting for bytes that never come, or reads part of the next response."))
    return s


def sec_headers():
    s = H2("5. Headers: the metadata layer")
    s.append(P("Headers are `Key: value` pairs that describe the message, not its content. **Why not put this in the URL or the body?** "
               "The video's parcel analogy is right: the courier needs the address written **on the outside** so every handler "
               "along the route (proxies, load balancers, caches, browsers) can act without opening the box. Headers let infrastructure "
               "that never understands your JSON still route, cache, compress, authenticate and log your traffic."))
    s.append(table([
        ["Category", "Examples", "Purpose"],
        ["Request", "`User-Agent`, `Authorization`, `Accept`, `Accept-Language`, `Accept-Encoding`, `Origin`, `Host`, `Cookie`, `If-None-Match`", "Tell the server who is asking and what the client prefers or can handle"],
        ["Response", "`Set-Cookie`, `Location`, `Retry-After`, `WWW-Authenticate`, `Allow`, `Server`", "Server instructions and extra info about the answer"],
        ["General (both)", "`Date`, `Cache-Control`, `Connection`, `Via`", "Metadata about the message or connection itself"],
        ["Representation (both)", "`Content-Type`, `Content-Length`, `Content-Encoding`, `Content-Language`, `ETag`, `Last-Modified`", "Describe the body: its type, size, encoding, version"],
        ["Security", "`Strict-Transport-Security`, `Content-Security-Policy`, `X-Frame-Options`, `X-Content-Type-Options`, `Referrer-Policy`, cookie flags", "Make browsers refuse risky behaviour"],
        ["CORS", "`Access-Control-Allow-Origin`, `-Allow-Methods`, `-Allow-Headers`, `-Max-Age`, `-Allow-Credentials`", "Cross-origin permission (section 7)"],
        ["Proxy/infra", "`X-Forwarded-For`, `X-Forwarded-Proto`, `X-Request-ID`, `Forwarded`", "Preserve client info across proxies; trace a request through systems"],
    ], [1.6, 3.8, 3.1]))
    s.append(P("**Two ideas from the video:** headers are **extensible** (you can invent `X-My-Header` and nothing in the protocol breaks) "
               "and act as a **remote control** (the client steers the server with `Accept`, `Authorization`, `If-None-Match`; the server steers "
               "the client with `Cache-Control`, `Set-Cookie`, `Location`)."))
    s.append(table([
        ["Security header", "What it stops", "Typical API value"],
        ["`Strict-Transport-Security`", "Protocol downgrade and cookie theft on plain HTTP", "`max-age=31536000; includeSubDomains`"],
        ["`Content-Security-Policy`", "XSS: restricts where scripts, styles, images may load from", "API: `default-src 'none'; frame-ancestors 'none'`"],
        ["`X-Content-Type-Options`", "MIME sniffing (a JSON file being run as a script)", "`nosniff`"],
        ["`X-Frame-Options`", "Clickjacking (your page inside an attacker's iframe). Modern form: CSP `frame-ancestors`", "`DENY`"],
        ["`Referrer-Policy`", "URLs with IDs leaking to other sites", "`no-referrer`"],
        ["`Set-Cookie` flags", "`HttpOnly`: JS cannot read it. `Secure`: HTTPS only. `SameSite`: limits cross-site sending (CSRF)", "`HttpOnly; Secure; SameSite=Lax`"],
    ], [2.6, 3.7, 2.9]))
    s.append(P("Here is the real middleware from your own repo that sets them. It is a good example of a **middleware**: a function that wraps a "
               "handler, does something before and/or after, and calls `next`:"))
    s.append(code(grab(f"{REPO}/internal/middleware/middleware.go", r"^func SecurityHeaders", r"^}"),
                  title="auctionEngine/internal/middleware/middleware.go"))
    s.append(callout("pitfall", ["**Header names are case-insensitive** (`content-type` equals `Content-Type`). Go canonicalizes them: use `r.Header.Get(\"content-type\")`, "
                                 "but read the map directly and you must use canonical form.",
                                 "Never trust `X-Forwarded-For` or `X-Forwarded-Proto` unless the request really came from your own proxy; anyone can send them. "
                                 "Never put secrets in a URL: URLs end up in logs, browser history and `Referer` headers. Put them in a header."]))
    return s
