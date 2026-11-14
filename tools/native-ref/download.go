package main

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

func download(url, dst string) error {
	return downloadRetry(url, dst, &http.Client{Timeout: 2 * time.Minute}, time.Sleep)
}

func downloadRetry(url, dst string, client *http.Client, sleep func(time.Duration)) error {
	for attempt := 0; ; attempt++ {
		err := downloadOnce(url, dst, client)
		if err == nil || attempt == 2 || !transientNetwork(err) {
			return err
		}
		sleep(time.Second << attempt)
	}
}

func transientNetwork(err error) bool {
	var pe *os.PathError
	if errors.As(err, &pe) {
		return false
	}
	var ne net.Error
	return errors.As(err, &ne) && (ne.Timeout() || ne.Temporary()) ||
		errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) ||
		transientWindowsNetwork(err) ||
		errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.ECONNABORTED) ||
		errors.Is(err, syscall.EPIPE) || errors.Is(err, syscall.ETIMEDOUT)
}

func downloadOnce(url, dst string, client *http.Client) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	if tok := os.Getenv("GITHUB_TOKEN"); tok != "" { // net/http drops it on the cross-host redirect
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	f, err := os.CreateTemp(filepath.Dir(dst), ".download-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name()) // failed attempts must not leave a truncated archive at dst
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), dst)
}
