package googleauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

func writeCredentials(t *testing.T, tokenURI string) string {
	t.Helper()
	body := `{"installed":{"client_id":"id","client_secret":"secret","auth_uri":"http://127.0.0.1:1/auth","token_uri":"` + tokenURI + `/token","redirect_uris":["http://localhost"]}}`
	path := filepath.Join(t.TempDir(), "credentials.json")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

func writeToken(t *testing.T, tok oauth2.Token) string {
	t.Helper()
	b, err := json.Marshal(tok)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "token.json")
	require.NoError(t, os.WriteFile(path, b, 0o600))
	return path
}

func TestClient(t *testing.T) {
	t.Run("valid files send bearer token", func(t *testing.T) {
		var got string
		api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got = r.Header.Get("Authorization")
		}))
		defer api.Close()

		creds := writeCredentials(t, api.URL)
		token := writeToken(t, oauth2.Token{
			AccessToken:  "acc",
			RefreshToken: "ref",
			TokenType:    "Bearer",
			Expiry:       time.Now().Add(time.Hour),
		})

		client, err := Client(t.Context(), creds, token)
		require.NoError(t, err)
		require.NotNil(t, client)

		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, api.URL, nil)
		require.NoError(t, err)
		resp, err := client.Do(req)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
		assert.Equal(t, "Bearer acc", got)
	})

	t.Run("missing credentials file", func(t *testing.T) {
		token := writeToken(t, oauth2.Token{AccessToken: "a", RefreshToken: "r"})
		_, err := Client(t.Context(), filepath.Join(t.TempDir(), "none.json"), token)
		require.Error(t, err)
	})

	t.Run("malformed credentials", func(t *testing.T) {
		creds := filepath.Join(t.TempDir(), "credentials.json")
		require.NoError(t, os.WriteFile(creds, []byte("{not json"), 0o600))
		token := writeToken(t, oauth2.Token{AccessToken: "a", RefreshToken: "r"})
		_, err := Client(t.Context(), creds, token)
		require.Error(t, err)
	})

	t.Run("token without refresh token", func(t *testing.T) {
		creds := writeCredentials(t, "http://127.0.0.1:1")
		token := writeToken(t, oauth2.Token{AccessToken: "a"})
		_, err := Client(t.Context(), creds, token)
		require.ErrorIs(t, err, ErrMissingRefreshToken)
	})
}

func callback(ctx context.Context, authURL, stateOverride string, done chan<- error) {
	u, err := url.Parse(authURL)
	if err != nil {
		done <- err
		return
	}
	q := u.Query()
	state := q.Get("state")
	if stateOverride != "" {
		state = stateOverride
	}
	target := q.Get("redirect_uri") + "?" + url.Values{"code": {"c"}, "state": {state}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		done <- err
		return
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		done <- err
		return
	}
	done <- resp.Body.Close()
}

func TestAuthorize(t *testing.T) {
	t.Run("saves token with refresh token", func(t *testing.T) {
		var verifier string
		tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := r.ParseForm(); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			verifier = r.PostForm.Get("code_verifier")
			w.Header().Set("Content-Type", "application/json")
			if _, err := w.Write([]byte(`{"access_token":"a","refresh_token":"r","token_type":"Bearer","expires_in":3600}`)); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		}))
		defer tokenSrv.Close()

		creds := writeCredentials(t, tokenSrv.URL)
		tokenFile := filepath.Join(t.TempDir(), "token.json")
		done := make(chan error, 1)

		err := Authorize(t.Context(), creds, tokenFile, func(authURL string) {
			go callback(t.Context(), authURL, "", done)
		})
		require.NoError(t, err)
		require.NoError(t, <-done)
		assert.NotEmpty(t, verifier)

		info, err := os.Stat(tokenFile)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

		b, err := os.ReadFile(tokenFile)
		require.NoError(t, err)
		var tok oauth2.Token
		require.NoError(t, json.Unmarshal(b, &tok))
		assert.Equal(t, "r", tok.RefreshToken)
	})

	t.Run("state mismatch", func(t *testing.T) {
		creds := writeCredentials(t, "http://127.0.0.1:1")
		tokenFile := filepath.Join(t.TempDir(), "token.json")
		done := make(chan error, 1)

		err := Authorize(t.Context(), creds, tokenFile, func(authURL string) {
			go callback(t.Context(), authURL, "wrong", done)
		})
		require.ErrorIs(t, err, ErrStateMismatch)
		require.NoError(t, <-done)
		assert.NoFileExists(t, tokenFile)
	})

	t.Run("context cancelled before callback", func(t *testing.T) {
		creds := writeCredentials(t, "http://127.0.0.1:1")
		tokenFile := filepath.Join(t.TempDir(), "token.json")
		ctx, cancel := context.WithCancel(t.Context())

		err := Authorize(ctx, creds, tokenFile, func(string) {
			cancel()
		})
		require.ErrorIs(t, err, context.Canceled)
		assert.NoFileExists(t, tokenFile)
	})
}
