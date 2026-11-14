package main

import (
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type brokenBody struct{}

func (brokenBody) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
func (brokenBody) Close() error             { return nil }

func TestTransientNetwork(t *testing.T) {
	for _, err := range []error{io.EOF, io.ErrUnexpectedEOF, syscall.ECONNRESET, &net.DNSError{IsTemporary: true}} {
		if !transientNetwork(err) {
			t.Errorf("network error not retried: %v", err)
		}
	}
	for _, err := range []error{os.ErrPermission, &os.PathError{Op: "write", Path: "archive", Err: syscall.ETIMEDOUT}} {
		if transientNetwork(err) {
			t.Errorf("local error retried: %v", err)
		}
	}
}

func TestDownloadRetry(t *testing.T) {
	for _, mode := range []string{"timeout", "truncated", "permanent", "http", "exhausted"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			var delays []time.Duration
			client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				calls++
				if calls == 1 || mode == "exhausted" {
					switch mode {
					case "timeout", "exhausted":
						return nil, &net.DNSError{Err: "timeout", IsTimeout: true}
					case "truncated":
						return &http.Response{StatusCode: 200, Body: brokenBody{}, Header: make(http.Header)}, nil
					case "permanent":
						return nil, errors.New("permanent transport failure")
					case "http":
						return &http.Response{StatusCode: 403, Status: "403 Forbidden", Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
					}
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("complete archive")), Header: make(http.Header)}, nil
			})}
			dir := t.TempDir()
			dst := filepath.Join(dir, "archive.tar.gz")
			err := downloadRetry("https://example.invalid/archive", dst, client, func(d time.Duration) { delays = append(delays, d) })
			wantCalls := 1
			var wantDelays []time.Duration
			if mode == "timeout" || mode == "truncated" {
				wantCalls, wantDelays = 2, []time.Duration{time.Second}
				got, readErr := os.ReadFile(dst)
				if err != nil || readErr != nil || string(got) != "complete archive" {
					t.Fatalf("download = %q, %v, read: %v", got, err, readErr)
				}
			} else {
				if mode == "exhausted" {
					wantCalls, wantDelays = 3, []time.Duration{time.Second, 2 * time.Second}
				}
				if err == nil {
					t.Fatal("expected failure")
				}
				if _, err := os.Stat(dst); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("failed download left destination: %v", err)
				}
			}
			if calls != wantCalls || !reflect.DeepEqual(delays, wantDelays) {
				t.Fatalf("calls=%d delays=%v; want %d %v", calls, delays, wantCalls, wantDelays)
			}
			files, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			for _, f := range files {
				if strings.HasPrefix(f.Name(), ".download-") {
					t.Fatalf("temporary download remains: %s", f.Name())
				}
			}
		})
	}
}
