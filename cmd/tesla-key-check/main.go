// Command tesla-key-check reports whether a domain is set up to enrol an
// application's public key in customers' vehicles.
//
// Before the pairing link at https://tesla.com/_ak/<domain> can work, the
// application's public key has to be reachable at a fixed well-known path on
// that domain, and has to be a key the vehicle can use. When it is not, the
// failure surfaces in the Tesla mobile app rather than here, with a message
// that does not say what is wrong. This checks the parts that can be checked
// from outside.
//
// It does not talk to Tesla, and it cannot confirm that the key is registered
// with the partner endpoint or that a vehicle will accept it.
package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/x509"
	"encoding/pem"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// KeyPath is where Tesla fetches a third-party application's public key.
const KeyPath = "/.well-known/appspecific/com.tesla.3p.public-key.pem"

// maxKeyBytes bounds the response body. A P-256 public key in PEM is under 200
// bytes; anything much larger is a web page, usually an error page or a login
// redirect served with status 200.
const maxKeyBytes = 64 * 1024

type status int

const (
	statusOK status = iota
	statusWarn
	statusFail
)

func (s status) String() string {
	switch s {
	case statusOK:
		return "ok  "
	case statusWarn:
		return "warn"
	default:
		return "FAIL"
	}
}

type check struct {
	name   string
	status status
	detail string
}

func (c check) String() string {
	return fmt.Sprintf("[%s] %-22s %s", c.status, c.name, c.detail)
}

// checkDomain runs every check that can be made from outside, in order, and
// stops at the first one that makes the rest meaningless.
func checkDomain(ctx context.Context, client *http.Client, domain string, want *ecdsa.PublicKey) []check {
	var checks []check
	add := func(name string, s status, format string, args ...interface{}) {
		checks = append(checks, check{name: name, status: s, detail: fmt.Sprintf(format, args...)})
	}

	if detail, ok := validDomain(domain); !ok {
		add("domain", statusFail, "%s", detail)
		return checks
	}
	add("domain", statusOK, "%s", domain)

	target := "https://" + domain + KeyPath
	body, final, err := fetch(ctx, client, target)
	if err != nil {
		add("fetch", statusFail, "%s: %s", target, err)
		return checks
	}
	add("fetch", statusOK, "%s", target)

	if final != domain {
		add("redirect", statusWarn,
			"redirected to %s; Tesla follows the original URL, so serve the key on %s itself", final, domain)
	}

	block, _ := pem.Decode(body)
	if block == nil {
		add("pem", statusFail, "not a PEM block (%s)", describe(body))
		return checks
	}
	if block.Type != "PUBLIC KEY" {
		add("pem", statusFail, "PEM block is %q, want \"PUBLIC KEY\"", block.Type)
		return checks
	}
	add("pem", statusOK, "PUBLIC KEY block")

	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		add("key", statusFail, "cannot parse: %s", err)
		return checks
	}
	published, ok := parsed.(*ecdsa.PublicKey)
	if !ok {
		add("key", statusFail, "key is %T; vehicles only accept ECDSA prime256v1", parsed)
		return checks
	}
	if published.Curve != elliptic.P256() {
		add("key", statusFail, "curve is %s; vehicles only accept prime256v1 (P-256)", published.Curve.Params().Name)
		return checks
	}
	add("key", statusOK, "ECDSA prime256v1")

	if want != nil {
		if want.Equal(published) {
			add("matches local key", statusOK, "published key is the one supplied")
		} else {
			add("matches local key", statusFail, "published key differs from the one supplied")
		}
	}
	return checks
}

// validDomain rejects the inputs people reach for instead of a bare hostname.
// Tesla builds the URL itself, so anything but a host is a mistake, and a port
// is worth naming separately because it fails in a way that looks like a
// certificate problem.
func validDomain(domain string) (string, bool) {
	switch {
	case domain == "":
		return "no domain given", false
	case strings.Contains(domain, "://"):
		return "give the bare domain, without a scheme", false
	case strings.Contains(domain, "/"):
		return "give the bare domain, without a path", false
	case strings.Contains(domain, "@"):
		return "give the bare domain, without credentials", false
	case strings.Contains(domain, ":"):
		return "give the bare domain, without a port: Tesla fetches the key over HTTPS on 443", false
	case !strings.Contains(domain, "."):
		return "not a domain name", false
	}
	if _, err := url.Parse("https://" + domain); err != nil {
		return fmt.Sprintf("not a valid domain: %s", err), false
	}
	return "", true
}

// fetch returns the body and the host that finally served it.
func fetch(ctx context.Context, client *http.Client, target string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, "", err
	}
	rsp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = rsp.Body.Close() }()

	if rsp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("HTTP %d %s", rsp.StatusCode, http.StatusText(rsp.StatusCode))
	}
	body, err := io.ReadAll(&io.LimitedReader{R: rsp.Body, N: maxKeyBytes})
	if err != nil {
		return nil, "", err
	}
	return body, rsp.Request.URL.Host, nil
}

// describe summarises a body that is not a PEM block, so the operator can tell
// an error page from a truncated file without fetching it again.
func describe(body []byte) string {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return "empty response"
	}
	if strings.HasPrefix(strings.ToLower(trimmed), "<!doctype") || strings.HasPrefix(trimmed, "<html") {
		return "looks like an HTML page"
	}
	if len(trimmed) > 40 {
		trimmed = trimmed[:40] + "..."
	}
	return fmt.Sprintf("%d bytes starting %q", len(body), trimmed)
}

func loadPublicKey(filename string) (*ecdsa.PublicKey, error) {
	contents, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(contents)
	if block == nil {
		return nil, fmt.Errorf("%s is not a PEM file", filename)
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	key, ok := parsed.(*ecdsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("%s does not hold an ECDSA public key", filename)
	}
	return key, nil
}

func usage() {
	w := flag.CommandLine.Output()
	fmt.Fprintf(w, "usage: %s [-public-key file] domain\n\n", filepath.Base(os.Args[0]))
	fmt.Fprintf(w, "Checks that domain serves an application public key that vehicles can use,\n")
	fmt.Fprintf(w, "at %s.\n\n", KeyPath)
	fmt.Fprintf(w, "Give the bare domain, for example example.com, not a URL.\n")
	flag.PrintDefaults()
}

func main() {
	var publicKeyFile string
	flag.StringVar(&publicKeyFile, "public-key", "", "Compare the published key against this PEM `file`")
	flag.Usage = usage
	flag.Parse()

	if flag.NArg() != 1 {
		usage()
		os.Exit(2)
	}
	domain := flag.Arg(0)

	var want *ecdsa.PublicKey
	if publicKeyFile != "" {
		var err error
		if want, err = loadPublicKey(publicKeyFile); err != nil {
			fmt.Fprintf(os.Stderr, "Could not read %s: %s\n", publicKeyFile, err)
			os.Exit(1)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	checks := checkDomain(ctx, &http.Client{}, domain, want)
	failed := false
	for _, c := range checks {
		fmt.Println(c)
		if c.status == statusFail {
			failed = true
		}
	}

	if failed {
		fmt.Fprintf(os.Stderr, "\nVehicles cannot enrol this key until the failures above are fixed.\n")
		os.Exit(1)
	}
	fmt.Printf("\nPairing link: https://tesla.com/_ak/%s\n", domain)
	fmt.Printf("Use it exactly as shown; a trailing slash breaks the link.\n")
	fmt.Printf("This does not check that the key is registered with Tesla's partner endpoint.\n")
}
