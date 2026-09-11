// Standalone probe for the disposable integration stack, never the pilot.
package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"
)

func main() {
	if len(os.Args) < 3 || len(os.Args)%2 != 1 {
		panic("expected URL/status pairs")
	}
	c := &http.Client{Timeout: 4 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	for i := 1; i < len(os.Args); i += 2 {
		want, err := strconv.Atoi(os.Args[i+1])
		if err != nil {
			panic(err)
		}
		resp, err := c.Get(os.Args[i])
		if err != nil {
			panic(err)
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 256))
		resp.Body.Close()
		if err != nil || resp.StatusCode != want {
			panic(fmt.Sprintf("%s: status %d want %d", os.Args[i], resp.StatusCode, want))
		}
		if want == 503 && string(body) != "not ready\n" {
			panic("readiness leaked dependency details")
		}
		fmt.Printf("PASS %s: %d\n", os.Args[i], want)
	}
}
