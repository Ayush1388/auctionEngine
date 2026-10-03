from lib import *

CODE = "/home/user/auctionEngine/backend-course/code/ch05_http"
REPO = "/home/user/auctionEngine"


def sec_methods():
    s = H2("6. Methods: expressing intent")
    s.append(P("The method says **what you intend to do** with the resource at the URL. That matters because proxies, caches, browsers and "
               "retry logic treat methods differently without ever reading your code."))
    s.append(table([
        ["Method", "Intent", "Has body?", "Safe?", "Idempotent?", "Cacheable?", "Success code"],
        ["GET", "Read a resource", "No", "Yes", "Yes", "Yes", "200"],
        ["HEAD", "Like GET, headers only (check existence, size, ETag)", "No", "Yes", "Yes", "Yes", "200"],
        ["POST", "Create / run an action", "Yes", "No", "**No**", "Rarely", "201 (+ Location), 200, 202"],
        ["PUT", "**Replace** the whole resource at this URL", "Yes", "No", "Yes", "No", "200, 204 (201 if created)"],
        ["PATCH", "**Partially** update", "Yes", "No", "Not guaranteed", "No", "200, 204"],
        ["DELETE", "Remove", "Optional", "No", "Yes", "No", "204 (or 200)"],
        ["OPTIONS", "Ask what is allowed (CORS pre-flight)", "No", "Yes", "Yes", "No", "204"],
    ], [0.9, 3.2, 0.9, 0.7, 1.1, 0.9, 1.9]))
    s.append(P("**Safe** means \"does not change server state\" (so a crawler or prefetch may call it freely). **Idempotent** means \"calling it N "
               "times leaves the server in the same state as calling it once\"."))
    s += card("Idempotency",
              "A property of an operation: repeating the same request has the same effect on server state as sending it once.",
              "Networks fail. If a client times out it does not know whether the server processed the request. Idempotent operations can be retried blindly; non-idempotent ones risk a duplicate (two charges, two bids, two emails).",
              "Design every endpoint that may be retried (mobile apps, background workers, API gateways, payment and bidding calls) so a retry is harmless.",
              "Idempotent does not mean \"same response\": the 2nd `DELETE` returns 404 but the state (gone) is identical. Do not assume `PATCH` is idempotent (\"add 10 to balance\" is not).",
              "Safe retries, simple client code, resilient systems, can be built on automatic retry middleware.",
              "Extra work for POST: you need keys, storage with TTL, and care with concurrent duplicates.",
              "Use PUT with a client-chosen id, or POST with an `Idempotency-Key` header: the server stores the first result and replays it for repeats (code below).")
    s.append(sequence(["Client", "Server", "Key store"], [
        (0, 1, "POST /notes   Idempotency-Key: abc"), (1, 2, "seen 'abc'?"), (2, 1, "no", "res"), (1, 1, "create note, 201", "note"),
        (1, 2, "save abc → 201 + body"), (1, 0, "201 Created  (timeout! client never sees it)"),
        (0, 1, "RETRY POST /notes   Idempotency-Key: abc"), (1, 2, "seen 'abc'?"), (2, 1, "yes: saved response", "res"),
        (1, 0, "201 Created (replayed, no second note)")],
        caption="Figure 6.1  Idempotency key. The retry returns the stored answer; only one note exists.", lane_colors=[INK, RED, BLUE]))
    s.append(code(grab(f"{CODE}/server/idempotency.go", r"^func Idempotent", r"^}"), title="code/ch05_http/server/idempotency.go (tested)"))
    s.append(P("**PUT vs PATCH** in the sample server: `PUT` demands a complete representation and replaces it. `PATCH` uses pointer fields so "
               "\"field absent\" is different from \"field set to empty\":"))
    s.append(code(grab(f"{CODE}/server/handlers.go", r"PATCH changes only", r"^}"), title="code/ch05_http/server/handlers.go"))
    s.append(callout("gotcha", [
        "The video says: \"always use PATCH unless you have a specific reason to use PUT\". That is a fine rule of thumb for update endpoints. "
        "Add that PUT is also correct when the **client chooses the resource id** (create-or-replace, `PUT /users/me/avatar`), and that "
        "many large APIs deliberately offer only POST and PATCH.",
        "The video's CORS section says a request is pre-flighted if the method is \"PUT or DELETE\". More exactly: **anything other than GET, HEAD or POST**, "
        "which also includes PATCH."]))
    s.append(callout("interview", "\"Is DELETE idempotent if the second call returns 404?\" **Yes.** Idempotency is about the resulting state "
                                  "of the server, not about getting identical status codes."))
    return s


def sec_cors():
    s = H2("7. CORS, the same-origin policy and pre-flight")
    s.append(P("An **origin** is `scheme + host + port`. `http://localhost:5173` and `http://localhost:3000` are different origins; so are "
               "`https://a.com` and `https://api.a.com`. The browser's **same-origin policy** lets JavaScript on one origin send requests anywhere "
               "but blocks it from reading responses from another origin, unless that origin opts in with CORS headers."))
    s.append(callout("gotcha", [
        "**CORS is enforced by the browser, not the server.** The server only states a policy. `curl`, Postman, mobile apps and other servers ignore CORS entirely, "
        "so CORS is **not** protection for your API: you still need authentication. Also, with a simple request the request is "
        "still sent and executed; CORS only hides the response from the page. This is why CSRF is a separate problem.",
        "A \"CORS error\" in the console almost always means the **server response is missing a header** (or the pre-flight failed), not that the "
        "frontend code is wrong. Fix it on the server."]))
    s.append(decision(caption="Figure 7.1  How a browser decides between a simple request and a pre-flight request."))
    s.append(sequence(["Browser (app.com)", "API (api.com)"], [
        (0, 1, "GET /data   Origin: app.com"), (1, 0, "200  Access-Control-Allow-Origin: app.com"),
        (0, 0, "headers match → JS may read it", "note")],
        caption="Figure 7.2  Simple request. If the Allow-Origin header is missing or different, the browser hides the response (CORS error).",
        lane_colors=[INK, RED]))
    s.append(sequence(["Browser (app.com)", "API (api.com)"], [
        (0, 1, "OPTIONS /notes/1  Origin  ·  Request-Method: PUT  ·  Request-Headers: authorization"),
        (1, 0, "204  Allow-Origin · Allow-Methods: PUT,… · Allow-Headers: Authorization · Max-Age: 86400"),
        (0, 0, "browser checks every item, caches the answer", "note"),
        (0, 1, "PUT /notes/1   Authorization: Bearer …   {json}"), (1, 0, "200 OK   Access-Control-Allow-Origin: app.com")],
        caption="Figure 7.3  Pre-flight. Two round trips for the first call. Max-Age lets the browser skip the first one next time.",
        lane_colors=[INK, RED]))
    s.append(table([
        ["Pre-flight triggered when the request is cross-origin AND at least one is true", "Example"],
        ["Method is not GET, HEAD or POST", "PUT, PATCH, DELETE"],
        ["It carries a non-safelisted header", "`Authorization`, `X-Request-ID`, `Idempotency-Key`"],
        ["`Content-Type` is not form-urlencoded, multipart/form-data or text/plain", "`application/json` (so **most JSON APIs always pre-flight**)"],
    ], [5, 3.5]))
    s.append(code(grab(f"{CODE}/server/middleware.go", r"^func CORS", r"^}"), title="code/ch05_http/server/middleware.go"))
    s.append(P("Your production version adds `Expose-Headers`, an `Idempotency-Key` allowance and a 403 for pre-flights from unknown origins:"))
    s.append(code(grab(f"{REPO}/internal/middleware/middleware.go", r"const \(", r"maxAge"), title="auctionEngine/internal/middleware/middleware.go"))
    s += card("CORS",
              "A set of `Access-Control-*` response headers by which a server tells browsers which other origins may read its responses and with which methods and headers.",
              "Without it any website you visit could read your logged-in data from other sites. It is the safe default that lets you opt in selectively.",
              "Whenever a browser frontend on one origin calls an API on another (SPA on `app.example.com`, API on `api.example.com`; Vite dev server on :5173 calling :8080).",
              "Do not use `*` for authenticated APIs. Do not echo back any `Origin` header blindly (that is the same as `*`). Not needed if the frontend and API are served from the same origin (put both behind one reverse proxy).",
              "Fine-grained, standard, cached by browsers via `Max-Age`.",
              "Extra round trip for pre-flighted calls; easy to misconfigure; confusing errors; says nothing about non-browser clients.",
              "Use an **allow-list** middleware as above, add `Vary: Origin`, and put it **before** authentication, because pre-flight requests carry no credentials.")
    s.append(callout("pitfall", [
        "`Access-Control-Allow-Origin: *` together with `Allow-Credentials: true` is **rejected by browsers**. With cookies you must echo one specific origin.",
        "Forgetting `Vary: Origin` lets a shared cache serve a response with the wrong `Allow-Origin` to another site.",
        "Only handling CORS on success: error responses (401, 404, 500) also need the headers or the frontend sees a CORS error instead of the real error.",
        "Browsers cap `Max-Age`: Chromium about 2 hours, Firefox 24 hours, whatever you send. In the video's demo it was set to 0 only for testing.",
        "Auth middleware placed before CORS rejects the pre-flight with 401, and the browser reports a CORS failure."]))
    s.append(callout("video", "The video demonstrates this with **Burp Suite** (an intercepting proxy for security testing). You can see the same "
                             "thing for free in the browser DevTools **Network** tab, or with `curl -i -X OPTIONS ... -H 'Origin: ...' "
                             "-H 'Access-Control-Request-Method: PUT'` against **your own** servers. Only intercept traffic of apps you own or are authorised to test."))
    return s


def sec_status():
    s = H2("8. Status codes: the universal result language")
    s.append(P("A status code lets a client decide what to do **without parsing the body**: retry, log the user out, show a form error, "
               "or stop. Because the codes are standard, a Go server, a Rust server and a Python server look the same to every client and proxy."))
    s.append(table([
        ["Range", "Meaning", "Client's reaction"],
        ["1xx", "Informational", "Keep going: `100 Continue`, `101 Switching Protocols` (WebSocket upgrade), `103 Early Hints`"],
        ["2xx", "Success", "Use the result"],
        ["3xx", "Redirection", "Look elsewhere or use your cache"],
        ["4xx", "**Client** error: the request is wrong", "Do not retry unchanged (except 408/429); fix the request"],
        ["5xx", "**Server** error: we failed", "Safe to retry later with back-off (if idempotent)"],
    ], [0.9, 3, 5.8]))
    s.append(table([
        ["Code", "Name", "Use it when", "Go constant"],
        ["200", "OK", "Successful GET/PUT/PATCH with a body", "`StatusOK`"],
        ["201", "Created", "POST created something; add a `Location` header", "`StatusCreated`"],
        ["202", "Accepted", "Work queued for later (background job); not done yet", "`StatusAccepted`"],
        ["204", "No Content", "Success with nothing to return (DELETE, pre-flight)", "`StatusNoContent`"],
        ["301", "Moved Permanently", "URL changed forever (browsers and search engines cache it)", "`StatusMovedPermanently`"],
        ["302", "Found", "Temporary redirect (campaign, login bounce)", "`StatusFound`"],
        ["304", "Not Modified", "Conditional GET matched: use your cached copy, no body", "`StatusNotModified`"],
        ["307 / 308", "Temp / Perm redirect", "Like 302/301 but the **method and body are preserved** (use for POST redirects)", "`StatusTemporaryRedirect`"],
        ["400", "Bad Request", "Malformed or invalid input (bad JSON, wrong type, failed validation)", "`StatusBadRequest`"],
        ["401", "Unauthorized", "Not authenticated: no, expired or invalid credentials. Send `WWW-Authenticate`", "`StatusUnauthorized`"],
        ["403", "Forbidden", "Authenticated but not allowed (user A deleting user B's data)", "`StatusForbidden`"],
        ["404", "Not Found", "No such resource (or you hide that it exists)", "`StatusNotFound`"],
        ["405", "Method Not Allowed", "Path exists, method does not. Send `Allow`. Go's mux does it for you", "`StatusMethodNotAllowed`"],
        ["406", "Not Acceptable", "Cannot produce any format the `Accept` header allows", "`StatusNotAcceptable`"],
        ["409", "Conflict", "State conflict: duplicate name, edit collision, outbid", "`StatusConflict`"],
        ["412", "Precondition Failed", "`If-Match` did not match (optimistic locking)", "`StatusPreconditionFailed`"],
        ["413", "Content Too Large", "Body over your limit", "`StatusRequestEntityTooLarge`"],
        ["415", "Unsupported Media Type", "You only accept JSON and got something else", "`StatusUnsupportedMediaType`"],
        ["422", "Unprocessable Content", "Syntax fine, meaning invalid (many APIs just use 400)", "`StatusUnprocessableEntity`"],
        ["429", "Too Many Requests", "Rate limited. Send `Retry-After`", "`StatusTooManyRequests`"],
        ["500", "Internal Server Error", "Unhandled failure. Log details, return a generic message", "`StatusInternalServerError`"],
        ["502", "Bad Gateway", "A proxy got an invalid reply from your app", "`StatusBadGateway`"],
        ["503", "Service Unavailable", "Overloaded, deploying or draining. Send `Retry-After`", "`StatusServiceUnavailable`"],
        ["504", "Gateway Timeout", "A proxy gave up waiting for your app", "`StatusGatewayTimeout`"],
    ], [0.8, 1.6, 4.3, 3.4], mono_cols=(3,)))
    s.append(sequence(["Client", "Nginx / LB", "Go app"], [
        (0, 1, "GET /report"), (1, 2, "forward"), (2, 2, "crashed / port closed", "note"),
        (1, 0, "502 Bad Gateway  (app answered garbage or refused)"),
        (0, 1, "GET /report (again)"), (1, 2, "forward"), (2, 2, "stuck for 60 s", "note"),
        (1, 0, "504 Gateway Timeout  (proxy gave up waiting)")],
        caption="Figure 8.1  502 vs 504. Your code rarely returns these; proxies do. Seeing them tells you the problem is the app or its connection.",
        lane_colors=[INK, TEAL, RED]))
    s.append(callout("gotcha", [
        "**401 vs 403.** 401's name says \"unauthorized\" but it means **unauthenticated** (\"I don't know who you are\"). 403 means \"I know who you are, and no\". "
        "The video explains this correctly; just memorise it as authn vs authz.",
        "**Hiding existence:** for private resources return **404** instead of 403 so you do not reveal that the id exists.",
        "**Validation errors:** the video uses 400. Both 400 and 422 are defensible; be consistent and always return a machine-readable body "
        "(`{\"error\":\"invalid input\",\"fields\":{\"email\":\"is required\"}}`, which is what your `httpx` package does).",
        "**301 vs 302 vs 308:** old browsers turned a redirected POST into a GET for 301/302. Use 307/308 when the method must be preserved."]))
    s.append(P("The consistent-errors helper from your own repo, the pattern every handler should use so errors look the same everywhere:"))
    s.append(code(grab(f"{REPO}/internal/httpx/httpx.go", r"^type errorResponse", r"^func ValidationError") + "\n\t…\n}",
                  title="auctionEngine/internal/httpx/httpx.go"))
    s.append(callout("pitfall", ["Returning **200 with `{\"success\": false}`** breaks every client, cache, monitor and retry policy. Use the real status code.",
                                 "Returning 500 for client mistakes (bad input) pollutes your error-rate alerts. Returning 200 for server crashes hides outages.",
                                 "Leaking internals in 500 bodies (stack traces, SQL) is an information-disclosure bug: log it, return a generic message."]))
    return s


def sec_caching():
    s = H2("9. HTTP caching: reuse what has not changed")
    s.append(P("Caching stores a copy of a response so the next request can skip work. Two mechanisms work together:"))
    s += bullets(["**Freshness** (`Cache-Control: max-age=N`): for N seconds the client reuses its copy **without asking the server at all**.",
                  "**Validation** (`ETag` + `If-None-Match`, or `Last-Modified` + `If-Modified-Since`): after the copy goes stale the client asks "
                  "\"has it changed?\". If not, the server answers `304 Not Modified` with **no body**, saving bandwidth."])
    s.append(sequence(["Browser cache", "Server"], [
        (0, 1, "GET /notes/1"), (1, 0, "200  ETag: \"v1\"  Cache-Control: max-age=10  + body"),
        (0, 0, "within 10 s: reuse copy, no request", "note"),
        (0, 1, "GET /notes/1   If-None-Match: \"v1\"   (after 10 s)"), (1, 0, "304 Not Modified  (no body)"),
        (0, 1, "…resource edited on server…", "note"),
        (0, 1, "GET /notes/1   If-None-Match: \"v1\""), (1, 0, "200  ETag: \"v2\"  + new body")],
        caption="Figure 9.1  The caching conversation from the video's demo (the video used 10 s max-age and numeric ETags).",
        lane_colors=[INK, RED]))
    s.append(code(grab(f"{CODE}/server/handlers.go", r"^// GET is safe", r"^}"), title="code/ch05_http/server/handlers.go"))
    s.append(code("""$ curl -si localhost:8082/notes/1 | grep -iE "^HTTP|etag|cache-control|last-mod"
HTTP/1.1 200 OK
Cache-Control: max-age=10
Etag: "1aa11812ea0bfb60"
Last-Modified: Sat, 03 Oct 2026 16:23:23 GMT

$ curl -si -H 'If-None-Match: "1aa11812ea0bfb60"' localhost:8082/notes/1 | head -1
HTTP/1.1 304 Not Modified""", lang="text", title="Real output from the server in this PDF's repo"))
    s.append(table([
        ["Directive", "Meaning", "Typical use"],
        ["`max-age=N`", "Fresh for N seconds in any cache", "Static assets, rarely changing data"],
        ["`s-maxage=N`", "Like max-age but only for shared caches (CDN)", "CDN caches longer than browsers"],
        ["`no-cache`", "**Stored, but must revalidate before every use** (name is misleading)", "Data that must be current but can be 304"],
        ["`no-store`", "**Never store** anywhere", "Banking pages, tokens, personal data"],
        ["`private` / `public`", "Only the browser may cache / any cache may cache", "`private` for user-specific responses"],
        ["`immutable`", "Will never change at this URL", "Fingerprinted files like `app.3f9a1c.js`"],
        ["`stale-while-revalidate=N`", "Serve stale instantly while refreshing in the background", "Fast UI for slightly stale data"],
        ["`Vary: Accept-Encoding`", "The cache key also includes these request headers", "Required whenever content negotiation changes the body"],
    ], [2.6, 3.8, 2.8]))
    s += card("HTTP caching (ETag, Cache-Control, Last-Modified)",
              "Browsers, proxies and CDNs keep copies of responses and reuse them when the rules in the headers allow, or revalidate cheaply with a conditional request.",
              "Fewer bytes over the network, lower latency (zero round trips while fresh), lower server and database load.",
              "Public or rarely changing data: static files, product catalogues, images, configuration, public API reads.",
              "User-specific or sensitive data (use `private` or `no-store`). Data that must be exact in real time (live bid price). Anything where a stale copy causes harm.",
              "Free and built into every browser and CDN; huge savings; works for any client that speaks HTTP.",
              "Staleness is hard to fix (you cannot push-invalidate a browser cache); easy to forget to change the ETag; the wrong `Vary` serves one user another's data.",
              "Generate a strong ETag from the representation (hash) or a version number, honour `If-None-Match` before doing expensive work, always set an explicit `Cache-Control`.")
    s.append(callout("gotcha", [
        "The video says client-side libraries (React Query/TanStack Query) are \"better\" than HTTP caching. They solve a different layer: they give the app control "
        "of when to refetch, while HTTP caching is understood by **every** intermediary and costs the client nothing. Real systems use both, plus server-side caching "
        "(Redis, cache-aside) which is Part 6.",
        "Without any `Cache-Control`, browsers may still cache using a **heuristic** based on `Last-Modified`. Always be explicit.",
        "`ETag` is usually a hash of the response, but a version column or `updated_at` works too. Weak ETags (`W/\"…\"`) mean \"semantically the same\"."]))
    s.append(callout("interview", "\"Users see old data after a deploy.\" → Check `Cache-Control` on the HTML and JS. Fix: no-cache on the HTML entry point, long `max-age` + "
                                  "`immutable` on content-hashed asset filenames, so a new deploy changes the URLs."))
    return s


def sec_negotiation():
    s = H2("10. Content negotiation and compression")
    s.append(P("The client states what it can handle and prefers; the server picks the best match and **says what it chose**. Three axes:"))
    s.append(table([
        ["Axis", "Request header", "Response header", "Example"],
        ["Media type", "`Accept: application/json`", "`Content-Type`", "JSON vs XML vs HTML"],
        ["Language", "`Accept-Language: es-ES,es;q=0.9`", "`Content-Language`", "Hola vs Hello"],
        ["Encoding", "`Accept-Encoding: gzip, br`", "`Content-Encoding`", "Compress the body"],
    ], [1.2, 3.2, 2, 2.6]))
    s.append(code(grab(f"{CODE}/server/handlers.go", r"^// negotiate picks", r"^}"), title="code/ch05_http/server/handlers.go"))
    s.append(code("""$ curl -s -H 'Accept-Language: es' localhost:8082/hello
{"lang":"es","message":"Hola"}
$ curl -s -H 'Accept-Language: hi' localhost:8082/hello
{"lang":"hi","message":"Namaste"}
$ curl -si localhost:8082/notes/1 -H 'Accept: image/png' | head -1
HTTP/1.1 406 Not Acceptable""", lang="text", title="Real output"))
    s += card("Content negotiation",
              "Client and server agree on the representation of a resource (format, language, encoding) using `Accept*` request headers and `Content-*` response headers.",
              "One URL can serve many clients: mobile app wants compact JSON, a legacy partner wants XML, users in Spain want Spanish text.",
              "Public APIs with several formats, internationalised content, supporting compression.",
              "Most internal JSON APIs: just return JSON and answer 406/415 for anything else. Do not build five formats you will never test.",
              "Flexible without extra URLs; standard; enables compression.",
              "More code paths and tests; caches must key on `Vary`; clients often send sloppy `Accept` headers (`*/*`).",
              "Parse `Accept` (with q-values in real code), set `Vary: Accept`, answer 406 when nothing matches.")
    s.append(Paragraph("Compression", S["h3"]))
    s.append(P("The server compresses the body (gzip, or Brotli `br`, which compresses text better but costs more CPU) and the client decompresses it transparently. "
               "Measured on this PDF's sample server for the same 50,000-row JSON response:"))
    s.append(bars([("No compression", 2577, RED), ("gzip", 129, GREEN)], unit="KB",
                  caption="Figure 10.1  2,638,891 bytes → 131,744 bytes (about 20× smaller). Real numbers from `curl -s /big | wc -c`."))
    s.append(code(grab(f"{CODE}/server/middleware.go", r"^// Gzip compresses", r"^func \(g \*gzipWriter\) Write.*"), title="code/ch05_http/server/middleware.go"))
    s.append(callout("whennot", [
        "Do **not** compress already-compressed data (JPEG, PNG, MP4, ZIP): wasted CPU, sometimes larger output. Skip tiny responses (under about 1 KB).",
        "Compressing responses that mix **secrets with attacker-controlled input** over HTTPS can enable the **BREACH** attack; keep CSRF tokens out of compressible, "
        "reflected pages or mask them.",
        "In production, usually let the reverse proxy or CDN compress, not your Go code. Go's client transport automatically sends `Accept-Encoding: gzip` and "
        "decompresses, unless you set that header yourself."]))
    return s


def sec_keepalive_large():
    s = H2("11. Persistent connections, large requests and streaming")
    s.append(Paragraph("Keep-alive (persistent connections)", S["h3"]))
    s.append(P("Opening a connection costs one TCP round trip, plus one or two for TLS. HTTP/1.0 paid that for **every** request. HTTP/1.1 keeps the connection open and "
               "reuses it by default (`Connection: keep-alive` is the default; `Connection: close` ends it). HTTP/2 goes further and multiplexes many requests on one connection."))
    s += card("Persistent connections",
              "One TCP (and TLS) connection carries many request-response pairs in sequence until either side closes it or it sits idle too long.",
              "Skips repeated TCP and TLS handshakes; avoids exhausting ports and file descriptors; far lower latency for sequences of calls.",
              "Always, by default. Especially for service-to-service calls and anything with many small requests.",
              "Idle connections hold memory and file descriptors on the server: set `IdleTimeout`. Behind load balancers, align your idle timeout with theirs to avoid resets.",
              "Big latency and CPU savings with no code.",
              "Connections tie up server resources; a single slow response can block later ones on that HTTP/1.1 connection; stale reused connections can fail once.",
              "Server: set `IdleTimeout`. **Client**: reuse one shared `http.Client` (never create one per call), close response bodies, tune `MaxIdleConnsPerHost` (default 2).")
    s.append(code(grab(f"{CODE}/client/client.go", r"^func newClient", r"^}") + "\n\n" + grab(f"{CODE}/client/client.go", r"^func getJSON", r"^}"),
                  title="code/ch05_http/client/client.go"))
    s.append(code(grab(f"{CODE}/client/client_test.go", r"^// Five sequential", r"^}"), title="…and the test that proves 5 requests use 1 TCP connection"))
    s.append(callout("pitfall", ["`http.Get(url)` uses `http.DefaultClient`, which has **no timeout**: one slow upstream can hang goroutines forever and cascade into an outage.",
                                 "Forgetting `resp.Body.Close()` leaks connections. Not reading the body to the end prevents reuse. The cure is `defer resp.Body.Close()` and draining on error paths.",
                                 "Creating a new `http.Client` per request throws away the connection pool."]))
    s.append(Paragraph("Large requests: multipart/form-data", S["h3"]))
    s.append(P("A file does not fit a JSON body (it would need Base64, which is 33% bigger and must sit in memory). `multipart/form-data` splits the body into "
               "parts separated by a **boundary** string declared in `Content-Type`. Each part has its own headers (field name, filename, type):"))
    s.append(code("""POST /upload HTTP/1.1
Content-Type: multipart/form-data; boundary=----X9f2
Content-Length: 5209

------X9f2
Content-Disposition: form-data; name="photo"; filename="car.jpg"
Content-Type: image/jpeg

<5000 bytes of binary data>
------X9f2--""", lang="text", title="A multipart request on the wire"))
    s.append(code(grab(f"{CODE}/server/handlers.go", r"^// upload reads", r"^}"), title="code/ch05_http/server/handlers.go  (streams parts, never loads the whole file)"))
    s.append(callout("whennot", ["Enforce limits: `http.MaxBytesReader` (hard ceiling), a file-type allow-list by sniffing content (not by trusting the filename), and a random server-side file name.",
                                 "For very large files (video), do not proxy the bytes through your Go server at all: return a **pre-signed URL** so the client uploads straight to object storage (S3/GCS/R2). "
                                 "Resumable uploads and ranges are Part 9."]))
    s.append(Paragraph("Large responses: chunked transfer, SSE, and friends", S["h3"]))
    s.append(table([
        ["Technique", "How it works", "Direction", "Use it for", "Not for"],
        ["Plain response", "Content-Length known, one body", "Server → client", "Normal APIs", "Huge or endless data"],
        ["Chunked transfer (HTTP/1.1)", "No Content-Length; body sent in chunks, ends with a zero chunk. Go does it automatically when you `Flush()`", "Server → client", "Generated downloads, streaming JSON lines, exports", "Two-way chat"],
        ["Range requests (206)", "Client asks for bytes `Range: bytes=0-999`; `http.ServeContent` supports it", "Server → client", "Video seek, resumable downloads", "Dynamic data"],
        ["**Server-Sent Events**", "`Content-Type: text/event-stream`; long-lived response with `data: …\\n\\n` frames; browser `EventSource` auto-reconnects", "Server → client only", "Live notifications, progress bars, price ticks, logs", "Binary data; client → server messages"],
        ["WebSocket", "Starts as HTTP (`101 Switching Protocols`), then a two-way message channel", "Both", "Chat, games, collaborative editing, live auctions", "Simple one-way feeds (SSE is simpler)"],
        ["Long polling", "Client asks, server holds the request until data exists", "Server → client", "Legacy fallback", "New designs"],
    ], [1.5, 3.4, 1.1, 2.1, 1.5]))
    s.append(code(grab(f"{CODE}/server/handlers.go", r"^// stream pushes", r"^}"), title="code/ch05_http/server/handlers.go"))
    s.append(code("""$ curl -sN localhost:8082/stream
id: 1
data: chunk 1

id: 2
data: chunk 2
...""", lang="text", title="Real output: chunks arrive 200 ms apart over one open response"))
    s.append(callout("gotcha", [
        "The video lumps \"chunked transfer\" and \"text/event-stream\" together. They are different layers: **chunked encoding** is how HTTP/1.1 frames a body of unknown length "
        "(HTTP/2 has no chunked encoding; it uses DATA frames). **SSE** is a message format and browser API on top of any long-lived response.",
        "`http.Server.WriteTimeout` will cut off long streams. Leave it unset for streaming endpoints, or extend the deadline per request with `http.ResponseController` "
        "(Go 1.20+). Your repo's `LimitInFlight` already exempts `/v1/ws` for the same reason.",
        "Always select on `r.Context().Done()` so you stop working when the client disconnects. Nginx buffers by default: add `X-Accel-Buffering: no` for SSE."]))
    return s


def sec_tls():
    s = H2("12. HTTPS, TLS and certificates")
    s.append(P("**HTTPS = HTTP over TLS.** TLS provides three guarantees: **confidentiality** (nobody on the path can read it), **integrity** (nobody can modify it unnoticed) and "
               "**authentication** (you are really talking to `api.example.com`, proven by a certificate signed by a trusted authority). SSL is the obsolete predecessor; people still say "
               "\"SSL certificate\" out of habit."))
    s.append(sequence(["Client", "Server"], [
        (0, 1, "ClientHello: TLS versions, ciphers, key share"), (1, 0, "ServerHello + certificate + key share"),
        (0, 0, "verify cert chain + hostname + expiry", "note"),
        (0, 1, "Finished (both now share a session key)"), (1, 0, "Finished"),
        (0, 1, "encrypted HTTP request / response ⇄")],
        caption="Figure 12.1  Simplified TLS 1.3 handshake. Asymmetric crypto agrees a key; fast symmetric crypto then protects the data.",
        lane_colors=[INK, RED]))
    s += card("HTTPS / TLS",
              "Encryption, integrity and server identity for HTTP, using certificates issued by a certificate authority (e.g. Let's Encrypt, free and automatic).",
              "Plain HTTP exposes passwords, tokens and cookies to anyone on the network, and lets them inject content. Browsers also hide powerful features (service workers, geolocation, HTTP/2 and 3) from plain HTTP.",
              "**Always** in production, including internal service-to-service traffic when it crosses networks you do not fully control.",
              "In local development plain HTTP is fine. Do not hand-roll crypto or disable certificate verification (`InsecureSkipVerify: true`) \"just to make it work\".",
              "Security, trust, SEO and browser features, HTTP/2 and HTTP/3 support.",
              "Handshake latency, certificate renewal operations, harder to debug traffic (use the proxy's logs or `SSLKEYLOGFILE`).",
              "Most teams **terminate TLS at a reverse proxy / load balancer** (Nginx, Caddy, cloud LB) and speak plain HTTP to the Go app inside a private network. Go can also do it directly with `ListenAndServeTLS` or `autocert`.")
    s.append(code("""// Option A: TLS inside Go
srv := &http.Server{Addr: ":443", Handler: h, TLSConfig: &tls.Config{MinVersion: tls.VersionTLS12}}
log.Fatal(srv.ListenAndServeTLS("fullchain.pem", "privkey.pem"))

// Option B (most common): Go listens on :8080 behind Nginx/Caddy/LB that handles TLS.
// Then trust X-Forwarded-Proto ONLY from that proxy, and send HSTS from the app or the proxy.""",
                  title="Two ways to do HTTPS"))
    s.append(callout("repo", "`SecurityHeaders(hsts bool)` in your repo sends `Strict-Transport-Security` only when `HSTS` is enabled for production. Sending it over plain "
                             "HTTP does nothing, and enabling it too early on a domain you cannot yet serve over HTTPS locks users out for a year."))
    return s
