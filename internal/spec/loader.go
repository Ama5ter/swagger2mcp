package spec

import (
    "context"
    "errors"
    "fmt"
    "io"
    "net/http"
    "net/url"
    "os"
    "path/filepath"
    "regexp"
    "strings"
    "time"

    openapi2 "github.com/getkin/kin-openapi/openapi2"
    "github.com/getkin/kin-openapi/openapi2conv"
    "github.com/getkin/kin-openapi/openapi3"
    "gopkg.in/yaml.v3"
)

// ErrorCode categorizes loader errors for clearer handling and messaging.
type ErrorCode string

const (
    InputError      ErrorCode = "InputError"
    NetworkError    ErrorCode = "NetworkError"
    ParseError      ErrorCode = "ParseError"
    ValidationError ErrorCode = "ValidationError"
    ConversionError ErrorCode = "ConversionError"
)

// SpecError is a structured error with optional location and JSON Pointer.
type SpecError struct {
    Code        ErrorCode
    Message     string
    Location    string // file path or URL
    JSONPointer string // e.g. "#/paths/~1pets/get"
    Cause       error
}

func (e *SpecError) Error() string { return e.Message }
func (e *SpecError) Unwrap() error { return e.Cause }

// Settings configures loader behavior.
type Settings struct {
    // HTTPTimeout bounds each HTTP request.
    HTTPTimeout time.Duration
    // MaxRetries for transient HTTP failures (>=500, 429, or network errors).
    MaxRetries int
    // BackoffBase is the base delay for exponential backoff.
    BackoffBase time.Duration
    // AllowFileRefs controls whether file:// refs are allowed for external references.
    // Default false, but automatically allowed when the root input is a local file
    // to enable typical multi-file specs.
    AllowFileRefs bool
}

// DefaultSettings returns recommended defaults.
func DefaultSettings() Settings {
    return Settings{
        HTTPTimeout: 10 * time.Second,
        MaxRetries:  3,
        BackoffBase: 200 * time.Millisecond,
        AllowFileRefs: false,
    }
}

// Option mutates Settings.
type Option func(*Settings)

func WithHTTPTimeout(d time.Duration) Option    { return func(s *Settings) { s.HTTPTimeout = d } }
func WithMaxRetries(n int) Option              { return func(s *Settings) { s.MaxRetries = n } }
func WithBackoffBase(d time.Duration) Option   { return func(s *Settings) { s.BackoffBase = d } }
func WithAllowFileRefs(allow bool) Option      { return func(s *Settings) { s.AllowFileRefs = allow } }

// Load reads, validates, and returns an OpenAPI v3 document. If the input
// is Swagger v2.0, it converts it to v3 via kin-openapi openapi2conv.
//
// input may be a filesystem path or an http/https URL. file:// URLs are blocked
// by default (use WithAllowFileRefs(true) when loading from local files and you
// want to permit file-based external refs).
func Load(ctx context.Context, input string, opts ...Option) (*openapi3.T, error) {
    if strings.TrimSpace(input) == "" {
        return nil, &SpecError{Code: InputError, Message: "spec: input is empty"}
    }

    settings := DefaultSettings()
    for _, opt := range opts {
        opt(&settings)
    }

    // Classify input as URL or file path.
    u, uerr := url.Parse(input)
    isURL := uerr == nil && u.Scheme != "" && u.Host != ""

    if isURL {
        scheme := strings.ToLower(u.Scheme)
        if scheme == "file" {
            return nil, &SpecError{Code: InputError, Message: "spec: file:// URLs are blocked by default", Location: input}
        }
        if scheme != "http" && scheme != "https" {
            return nil, &SpecError{Code: InputError, Message: fmt.Sprintf("spec: unsupported URL scheme %q (only http/https allowed)", scheme), Location: input}
        }

        // Fetch head bytes to detect version reliably.
        raw, fetchErr := fetchWithRetry(ctx, input, settings)
        if fetchErr != nil {
            return nil, &SpecError{Code: NetworkError, Message: fmt.Sprintf("fetch %s: %v", input, fetchErr), Location: input, Cause: fetchErr}
        }

        version, derr := detectSpecVersion(raw)
        if derr != nil {
            return nil, &SpecError{Code: ParseError, Message: derr.Error(), Location: input, Cause: derr}
        }

        switch version {
        case 3:
            // Use loader with proper base URL support and external refs policy.
            loader := newLoader(settings, false /*rootIsFile*/)
            doc, err := loader.LoadFromURI(u)
            if err != nil {
                return nil, mapValidateOrParseErr(err, input)
            }
            if err := doc.Validate(ctx); err != nil {
                if !canProceedDespiteValidation(err) {
                    return nil, mapValidateOrParseErr(err, input)
                }
                // proceed in permissive mode
            }
            return doc, nil
        case 2:
            // Preprocess incompatible v2 constructs to improve conversion success.
            if fixed, changed, _ := preprocessV2ForCompatibility(raw); changed {
                raw = fixed
            }
            // Convert v2 bytes to v3, then validate.
            v3doc, err := convertV2ToV3(raw)
            if err != nil {
                return nil, &SpecError{Code: ConversionError, Message: fmt.Sprintf("convert v2→v3: %v", err), Location: input, Cause: err}
            }
            // Store original v2 schema definitions for enhanced parsing
            SetV2SchemaDefinitions(v3doc, raw)
            // Resolve all refs immediately after conversion
            loader := newLoader(settings, false)
            if err := loader.ResolveRefsIn(v3doc, nil); err != nil {
                fmt.Printf("[WARN] Failed to resolve refs after conversion: %v\n", err)
            }
            if err := v3doc.Validate(ctx); err != nil {
                if !canProceedDespiteValidation(err) {
                    return nil, mapValidateOrParseErr(err, input)
                }
                // proceed in permissive mode
            }
            return v3doc, nil
        default:
            return nil, &SpecError{Code: ParseError, Message: "spec: unknown or unsupported OpenAPI/Swagger version", Location: input}
        }
    }

    // Treat as local filesystem path.
    abs, err := filepath.Abs(input)
    if err != nil {
        return nil, &SpecError{Code: InputError, Message: fmt.Sprintf("resolve path: %v", err), Location: input, Cause: err}
    }

    // Read file to detect version.
    raw, rerr := os.ReadFile(abs)
    if rerr != nil {
        return nil, &SpecError{Code: InputError, Message: fmt.Sprintf("read file %s: %v", abs, rerr), Location: abs, Cause: rerr}
    }

    version, derr := detectSpecVersion(raw)
    if derr != nil {
        return nil, &SpecError{Code: ParseError, Message: derr.Error(), Location: abs, Cause: derr}
    }

    switch version {
    case 3:
        loader := newLoader(settings, true /*rootIsFile*/)
        doc, err := loader.LoadFromFile(abs)
        if err != nil {
            return nil, mapValidateOrParseErr(err, abs)
        }
        if err := doc.Validate(ctx); err != nil {
            if !canProceedDespiteValidation(err) {
                return nil, mapValidateOrParseErr(err, abs)
            }
            // proceed in permissive mode
        }
        return doc, nil
    case 2:
        // Preprocess incompatible v2 constructs to improve conversion success.
        if fixed, changed, _ := preprocessV2ForCompatibility(raw); changed {
            raw = fixed
        }
        v3doc, err := convertV2ToV3(raw)
        if err != nil {
            return nil, &SpecError{Code: ConversionError, Message: fmt.Sprintf("convert v2→v3: %v", err), Location: abs, Cause: err}
        }
        // Store original v2 schema definitions for enhanced parsing
        SetV2SchemaDefinitions(v3doc, raw)
        if err := v3doc.Validate(ctx); err != nil {
            if !canProceedDespiteValidation(err) {
                return nil, mapValidateOrParseErr(err, abs)
            }
            // proceed in permissive mode
        }
        return v3doc, nil
    default:
        return nil, &SpecError{Code: ParseError, Message: "spec: unknown or unsupported OpenAPI/Swagger version", Location: abs}
    }
}

func newLoader(settings Settings, rootIsFile bool) *openapi3.Loader {
    loader := openapi3.NewLoader()
    loader.IsExternalRefsAllowed = true
    client := &http.Client{Timeout: settings.HTTPTimeout}
    // Allow file refs only when configured or when loading from a local file root.
    allowFile := settings.AllowFileRefs || rootIsFile
    loader.ReadFromURIFunc = func(l *openapi3.Loader, uri *url.URL) ([]byte, error) {
        switch strings.ToLower(uri.Scheme) {
        case "", "file":
            if !allowFile {
                return nil, fmt.Errorf("blocked file ref: %s", uri.String())
            }
            // Read local file path
            path := uri.Path
            if path == "" {
                path = uri.Opaque
            }
            return os.ReadFile(path)
        case "http", "https":
            req, err := http.NewRequest("GET", uri.String(), nil)
            if err != nil {
                return nil, err
            }
            resp, err := client.Do(req)
            if err != nil {
                return nil, err
            }
            defer resp.Body.Close()
            if resp.StatusCode >= 400 {
                return nil, fmt.Errorf("http %d: %s", resp.StatusCode, uri.String())
            }
            return io.ReadAll(resp.Body)
        default:
            return nil, fmt.Errorf("unsupported ref scheme: %s", uri.Scheme)
        }
    }
    return loader
}

// detectSpecVersion returns 3 for OpenAPI v3, 2 for Swagger v2, else error.
func detectSpecVersion(data []byte) (int, error) {
    var root map[string]any
    if err := yaml.Unmarshal(data, &root); err != nil {
        return 0, fmt.Errorf("parse spec: %w", err)
    }
    // Check OpenAPI v3 key
    if v, ok := root["openapi"]; ok {
        if s, _ := v.(string); strings.HasPrefix(strings.TrimSpace(s), "3.") {
            return 3, nil
        }
    }
    // Check Swagger v2 key
    if v, ok := root["swagger"]; ok {
        if s, _ := v.(string); strings.HasPrefix(strings.TrimSpace(s), "2.") {
            return 2, nil
        }
    }
    return 0, fmt.Errorf("spec: missing or unknown version (expected 'openapi: 3.x' or 'swagger: 2.0')")
}

func convertV2ToV3(data []byte) (*openapi3.T, error) {
    // For kin-openapi v0.116.0, convert by unmarshalling to v2 then calling ToV3.
    var v2 openapi2.T
    if err := yaml.Unmarshal(data, &v2); err != nil {
        return nil, err
    }
    return openapi2conv.ToV3(&v2)
}


func fetchWithRetry(ctx context.Context, rawURL string, settings Settings) ([]byte, error) {
    client := &http.Client{Timeout: settings.HTTPTimeout}
    var lastErr error
    backoff := settings.BackoffBase
    if backoff <= 0 {
        backoff = 200 * time.Millisecond
    }
    attempts := settings.MaxRetries
    if attempts <= 0 {
        attempts = 1
    }
    for i := 0; i < attempts; i++ {
        req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
        if err != nil {
            return nil, err
        }
        resp, err := client.Do(req)
        if err == nil && resp != nil && resp.StatusCode < 300 {
            defer resp.Body.Close()
            return io.ReadAll(resp.Body)
        }
        if err != nil {
            lastErr = err
        } else {
            // HTTP error
            defer resp.Body.Close()
            if resp.StatusCode >= 500 || resp.StatusCode == 429 {
                lastErr = fmt.Errorf("transient http error %d", resp.StatusCode)
            } else {
                body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
                return nil, fmt.Errorf("http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
            }
        }
        // Backoff before next attempt
        select {
        case <-ctx.Done():
            return nil, ctx.Err()
        case <-time.After(backoff):
        }
        backoff *= 2
    }
    if lastErr == nil {
        lastErr = errors.New("fetch failed")
    }
    return nil, lastErr
}

func mapValidateOrParseErr(err error, location string) error {
    // Try to extract JSON Pointer where available.
    pointer := extractJSONPointer(err)
    code := ValidationError
    // Heuristics: some loader errors are parse errors.
    if strings.Contains(strings.ToLower(err.Error()), "parse") || strings.Contains(strings.ToLower(err.Error()), "invalid character") {
        code = ParseError
    }
    return &SpecError{Code: code, Message: err.Error(), Location: location, JSONPointer: pointer, Cause: err}
}

var jsonPtrRe = regexp.MustCompile(`#/[^\s'\"]+`)

func extractJSONPointer(err error) string {
    if err == nil {
        return ""
    }
    // Unwrap MultiError and take the first for brevity.
    if me, ok := err.(openapi3.MultiError); ok {
        if len(me) > 0 {
            return extractJSONPointer(me[0])
        }
    }
    var se *openapi3.SchemaError
    if errors.As(err, &se) {
        // v0.116 uses JSONPointer() []string
        if parts := se.JSONPointer(); len(parts) > 0 {
            // Build a JSON pointer path
            return "#/" + strings.Join(parts, "/")
        }
        if se.SchemaField != "" {
            return se.SchemaField
        }
    }
    // Fallback: parse from error message if a pointer literal appears.
    msg := err.Error()
    if m := jsonPtrRe.FindString(msg); m != "" {
        return m
    }
    return ""
}

// canProceedDespiteValidation returns true for certain validation errors where
// a best-effort build can still proceed (e.g., unresolved $ref entries).
func canProceedDespiteValidation(err error) bool {
    if err == nil { return true }
    s := strings.ToLower(err.Error())
    if strings.Contains(s, "unresolved ref") || strings.Contains(s, "found unresolved ref") {
        return true
    }
    return false
}
