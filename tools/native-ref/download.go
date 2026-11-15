package main

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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
	if tok := os.Getenv("GITHUB_TOKEN"); tok != "" && (req.URL.Hostname() == "api.github.com" || req.URL.Hostname() == "github.com") { // net/http drops it on the cross-host redirect
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

// downloadVerified never accepts a mirror response without the original archive pin.
func downloadVerified(upstream, dst, sum string, mirrors []string) error {
	urls := []string{upstream}
	for _, base := range mirrors {
		urls = append(urls, strings.TrimRight(base, "/")+"/"+filepath.Base(dst))
	}
	var failures []error
	for _, url := range urls {
		err := download(url, dst)
		if err == nil {
			got, hashErr := fileSHA(dst)
			err = hashErr
			if err == nil && got != sum {
				err = fmt.Errorf("GET %s: sha256 %s, pinned %s", url, got, sum)
			}
			if err == nil {
				return nil
			}
			if removeErr := os.Remove(dst); removeErr != nil {
				return errors.Join(err, removeErr)
			}
		}
		failures = append(failures, err)
		fmt.Fprintf(os.Stderr, "fetch: %s failed; trying next pinned mirror if available\n", url)
	}
	return errors.Join(failures...)
}
