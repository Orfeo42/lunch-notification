package googleauth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/sheets/v4"
	"google.golang.org/api/tasks/v1"
)

type sentinelError string

func (e sentinelError) Error() string { return string(e) }

const (
	ErrMissingRefreshToken = sentinelError("token has no refresh token")
	ErrStateMismatch       = sentinelError("oauth state mismatch")
	ErrMissingCode         = sentinelError("oauth callback has no code")
)

const (
	tokenFilePerm   = 0o600
	readHeaderLimit = 10 * time.Second
	shutdownLimit   = 5 * time.Second
	stateBytes      = 32
)

type callbackResult struct {
	code string
	err  error
}

func scopes() []string {
	return []string{tasks.TasksScope, sheets.SpreadsheetsReadonlyScope}
}

func loadConfig(credentialsFile string) (*oauth2.Config, error) {
	b, err := os.ReadFile(credentialsFile)
	if err != nil {
		return nil, fmt.Errorf("read credentials file: %w", err)
	}
	cfg, err := google.ConfigFromJSON(b, scopes()...)
	if err != nil {
		return nil, fmt.Errorf("parse credentials file: %w", err)
	}
	return cfg, nil
}

func Client(ctx context.Context, credentialsFile, tokenFile string) (*http.Client, error) {
	cfg, err := loadConfig(credentialsFile)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(tokenFile)
	if err != nil {
		return nil, fmt.Errorf("read token file: %w", err)
	}
	var tok oauth2.Token
	if err := json.Unmarshal(b, &tok); err != nil {
		return nil, fmt.Errorf("parse token file: %w", err)
	}
	if tok.RefreshToken == "" {
		return nil, ErrMissingRefreshToken
	}
	return oauth2.NewClient(ctx, cfg.TokenSource(ctx, &tok)), nil
}

func Authorize(ctx context.Context, credentialsFile, tokenFile string, prompt func(authURL string)) error {
	cfg, err := loadConfig(credentialsFile)
	if err != nil {
		return err
	}
	raw := make([]byte, stateBytes)
	if _, err := rand.Read(raw); err != nil {
		return fmt.Errorf("generate state: %w", err)
	}
	state := base64.RawURLEncoding.EncodeToString(raw)
	verifier := oauth2.GenerateVerifier()

	var lc net.ListenConfig
	listener, err := lc.Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("listen on loopback: %w", err)
	}
	cfg.RedirectURL = "http://" + listener.Addr().String()

	results := make(chan callbackResult, 1)
	srv := &http.Server{
		ReadHeaderTimeout: readHeaderLimit,
		Handler:           callbackHandler(state, results),
	}
	serveErrs := make(chan error, 1)
	go func() {
		serveErrs <- srv.Serve(listener)
	}()

	prompt(cfg.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce, oauth2.S256ChallengeOption(verifier)))

	var result callbackResult
	select {
	case result = <-results:
	case err := <-serveErrs:
		result.err = fmt.Errorf("serve callback: %w", err)
	case <-ctx.Done():
		result.err = fmt.Errorf("wait for callback: %w", ctx.Err())
	}

	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownLimit)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown callback server: %w", err)
	}
	if result.err != nil {
		return result.err
	}

	tok, err := cfg.Exchange(ctx, result.code, oauth2.VerifierOption(verifier))
	if err != nil {
		return fmt.Errorf("exchange code: %w", err)
	}
	if tok.RefreshToken == "" {
		return ErrMissingRefreshToken
	}
	b, err := json.Marshal(tok)
	if err != nil {
		return fmt.Errorf("encode token: %w", err)
	}
	if err := os.WriteFile(tokenFile, b, tokenFilePerm); err != nil {
		return fmt.Errorf("write token file: %w", err)
	}
	if err := os.Chmod(tokenFile, tokenFilePerm); err != nil {
		return fmt.Errorf("chmod token file: %w", err)
	}
	slog.InfoContext(ctx, "token saved", slog.String("path", tokenFile))
	return nil
}

func callbackHandler(state string, results chan<- callbackResult) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("state") != state {
			http.Error(w, "state mismatch", http.StatusBadRequest)
			offer(results, callbackResult{err: fmt.Errorf("callback: %w", ErrStateMismatch)})
			return
		}
		code := q.Get("code")
		if code == "" {
			http.Error(w, "missing code", http.StatusBadRequest)
			offer(results, callbackResult{err: fmt.Errorf("callback: %w", ErrMissingCode)})
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("Authorization complete, you can close this tab.")); err != nil {
			offer(results, callbackResult{err: fmt.Errorf("write callback response: %w", err)})
			return
		}
		offer(results, callbackResult{code: code})
	})
}

func offer(results chan<- callbackResult, r callbackResult) {
	select {
	case results <- r:
	default:
	}
}
