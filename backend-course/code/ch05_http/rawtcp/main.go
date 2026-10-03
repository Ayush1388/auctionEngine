// Chapter 5 (video 5): HTTP is just text over a TCP connection.
// This server speaks HTTP/1.1 by hand so you can see there is no magic.
//
//	go run ./ch05_http/rawtcp         # in another terminal:
//	curl -i localhost:8081/hello
//	printf 'GET /hello HTTP/1.1\r\nHost: x\r\n\r\n' | nc localhost 8081
package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"strings"
)

func main() {
	ln, err := net.Listen("tcp", ":8081") // TCP = the reliable transport HTTP needs
	if err != nil {
		log.Fatal(err)
	}
	log.Println("raw HTTP/1.1 server on :8081")
	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Println("accept:", err)
			continue
		}
		go handle(conn) // one goroutine per connection
	}
}

func handle(conn net.Conn) {
	defer conn.Close()
	r := bufio.NewReader(conn)

	// 1. Request line: METHOD SP TARGET SP VERSION
	line, err := r.ReadString('\n')
	if err != nil {
		return
	}
	parts := strings.Fields(line)
	if len(parts) != 3 {
		respond(conn, 400, "Bad Request", "malformed request line\n")
		return
	}
	method, target := parts[0], parts[1]

	// 2. Headers: "Key: value" lines until the blank line.
	headers := map[string]string{}
	for {
		h, err := r.ReadString('\n')
		if err != nil {
			return
		}
		h = strings.TrimRight(h, "\r\n")
		if h == "" {
			break // blank line = headers are over, body (if any) starts
		}
		if k, v, ok := strings.Cut(h, ":"); ok {
			headers[strings.ToLower(k)] = strings.TrimSpace(v)
		}
	}

	// 3. Route by hand.
	switch {
	case method == "GET" && target == "/hello":
		respond(conn, 200, "OK", "hello from raw TCP, you are "+headers["user-agent"]+"\n")
	case target == "/hello":
		respond(conn, 405, "Method Not Allowed", "use GET\n")
	default:
		respond(conn, 404, "Not Found", fmt.Sprintf("no route for %s\n", target))
	}
}

func respond(conn net.Conn, code int, reason, body string) {
	// Status line, headers, blank line, body.
	// Content-Length tells the client where the body ends.
	fmt.Fprintf(conn, "HTTP/1.1 %d %s\r\n", code, reason)
	fmt.Fprint(conn, "Content-Type: text/plain; charset=utf-8\r\n")
	fmt.Fprintf(conn, "Content-Length: %d\r\n", len(body))
	fmt.Fprint(conn, "Connection: close\r\n\r\n")
	fmt.Fprint(conn, body)
}
