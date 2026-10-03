// A production-minded HTTP client. The zero-value http.Client has NO timeout
// and the default transport keeps only 2 idle connections per host, two of the
// most common causes of hung goroutines and slow services.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"time"
)

func newClient() *http.Client {
	dialer := &net.Dialer{Timeout: 3 * time.Second, KeepAlive: 30 * time.Second}
	tr := &http.Transport{
		DialContext:           dialer.DialContext,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   20, // default is 2: too low for a service calling one host a lot
		IdleConnTimeout:       90 * time.Second,
	}
	return &http.Client{Transport: tr, Timeout: 15 * time.Second} // overall cap per request
}

// getJSON shows the three rules of a correct client call:
//  1. pass a context so the caller can cancel
//  2. always close the body (or the connection cannot be reused)
//  3. check the status code before decoding
func getJSON(ctx context.Context, c *http.Client, url string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20)) // drain so the connection is reusable
		return fmt.Errorf("unexpected status %s", resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func main() {
	var out map[string]string
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := getJSON(ctx, newClient(), os.Args[1], &out); err != nil {
		log.Fatal(err)
	}
	fmt.Println(out)
}
