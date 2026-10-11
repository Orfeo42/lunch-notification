package gtasks

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	"google.golang.org/api/tasks/v1"

	"github.com/orfeo42/lunch-notification/internal/todo"
)

type fakeAPI struct {
	t            *testing.T
	listStatus   int
	insertStatus int
	pages        map[string]tasks.Tasks
	posted       []tasks.Task
	query        map[string]string
}

func newFakeAPI(t *testing.T) *fakeAPI {
	t.Helper()
	return &fakeAPI{
		t:            t,
		listStatus:   http.StatusOK,
		insertStatus: http.StatusOK,
		pages:        map[string]tasks.Tasks{},
		query:        map[string]string{},
	}
}

func (f *fakeAPI) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	rw.Header().Set("Content-Type", "application/json")
	if r.Method == http.MethodPost {
		var body tasks.Task
		assert.NoError(f.t, json.NewDecoder(r.Body).Decode(&body))
		f.posted = append(f.posted, body)
		rw.WriteHeader(f.insertStatus)
		if f.insertStatus != http.StatusOK {
			_, err := rw.Write([]byte(`{"error":{"code":500,"message":"boom"}}`))
			assert.NoError(f.t, err)
			return
		}
		assert.NoError(f.t, json.NewEncoder(rw).Encode(tasks.Task{Id: "new-id", Title: body.Title, Due: body.Due}))
		return
	}
	q := r.URL.Query()
	for _, k := range []string{"dueMin", "dueMax", "showCompleted", "showHidden"} {
		f.query[k] = q.Get(k)
	}
	if f.listStatus != http.StatusOK {
		rw.WriteHeader(f.listStatus)
		_, err := rw.Write([]byte(`{"error":{"code":500,"message":"boom"}}`))
		assert.NoError(f.t, err)
		return
	}
	assert.NoError(f.t, json.NewEncoder(rw).Encode(f.pages[q.Get("pageToken")]))
}

func newWriter(t *testing.T, api *fakeAPI) *Writer {
	t.Helper()
	srv := httptest.NewServer(api)
	t.Cleanup(srv.Close)
	svc, err := tasks.NewService(context.Background(), option.WithEndpoint(srv.URL+"/"), option.WithHTTPClient(srv.Client()))
	require.NoError(t, err)
	return New(svc, "list1")
}

func romeTask(t *testing.T) todo.Task {
	t.Helper()
	rome, err := time.LoadLocation("Europe/Rome")
	require.NoError(t, err)
	return todo.Task{
		Title: "Pranzo: polpettone",
		Notes: "notes",
		Due:   time.Date(2026, 10, 12, 0, 0, 0, 0, rome),
	}
}

func TestEnsure(t *testing.T) {
	t.Run("same title and date present skips insert", func(t *testing.T) {
		api := newFakeAPI(t)
		api.pages[""] = tasks.Tasks{Items: []*tasks.Task{{Title: "Pranzo: polpettone", Due: "2026-10-12T00:00:00.000Z"}}}
		w := newWriter(t, api)

		require.NoError(t, w.Ensure(context.Background(), romeTask(t)))
		assert.Empty(t, api.posted)
	})

	t.Run("absent task inserts once with preserved date", func(t *testing.T) {
		api := newFakeAPI(t)
		w := newWriter(t, api)

		require.NoError(t, w.Ensure(context.Background(), romeTask(t)))
		require.Len(t, api.posted, 1)
		assert.Equal(t, "Pranzo: polpettone", api.posted[0].Title)
		assert.Equal(t, "notes", api.posted[0].Notes)
		assert.Equal(t, "2026-10-12T00:00:00Z", api.posted[0].Due)
	})

	t.Run("sends list query parameters", func(t *testing.T) {
		api := newFakeAPI(t)
		w := newWriter(t, api)

		require.NoError(t, w.Ensure(context.Background(), romeTask(t)))
		assert.Equal(t, "2026-10-12T00:00:00Z", api.query["dueMin"])
		assert.Equal(t, "2026-10-13T00:00:00Z", api.query["dueMax"])
		assert.Equal(t, "true", api.query["showCompleted"])
		assert.Equal(t, "true", api.query["showHidden"])
	})

	t.Run("same title different date inserts", func(t *testing.T) {
		api := newFakeAPI(t)
		api.pages[""] = tasks.Tasks{Items: []*tasks.Task{{Title: "Pranzo: polpettone", Due: "2026-10-13T00:00:00.000Z"}}}
		w := newWriter(t, api)

		require.NoError(t, w.Ensure(context.Background(), romeTask(t)))
		assert.Len(t, api.posted, 1)
	})

	t.Run("match on second page skips insert", func(t *testing.T) {
		api := newFakeAPI(t)
		api.pages[""] = tasks.Tasks{
			Items:         []*tasks.Task{{Title: "other", Due: "2026-10-12T00:00:00.000Z"}},
			NextPageToken: "p2",
		}
		api.pages["p2"] = tasks.Tasks{Items: []*tasks.Task{{Title: "Pranzo: polpettone", Due: "2026-10-12T00:00:00.000Z"}}}
		w := newWriter(t, api)

		require.NoError(t, w.Ensure(context.Background(), romeTask(t)))
		assert.Empty(t, api.posted)
	})

	t.Run("completed task counts as present", func(t *testing.T) {
		api := newFakeAPI(t)
		api.pages[""] = tasks.Tasks{Items: []*tasks.Task{{Title: "Pranzo: polpettone", Due: "2026-10-12T00:00:00.000Z", Status: "completed"}}}
		w := newWriter(t, api)

		require.NoError(t, w.Ensure(context.Background(), romeTask(t)))
		assert.Empty(t, api.posted)
	})

	t.Run("list failure returns googleapi error", func(t *testing.T) {
		api := newFakeAPI(t)
		api.listStatus = http.StatusInternalServerError
		w := newWriter(t, api)

		err := w.Ensure(context.Background(), romeTask(t))
		require.Error(t, err)
		var gerr *googleapi.Error
		assert.True(t, errors.As(err, &gerr))
		assert.Empty(t, api.posted)
	})

	t.Run("insert failure returns error", func(t *testing.T) {
		api := newFakeAPI(t)
		api.insertStatus = http.StatusInternalServerError
		w := newWriter(t, api)

		err := w.Ensure(context.Background(), romeTask(t))
		require.Error(t, err)
		var gerr *googleapi.Error
		assert.True(t, errors.As(err, &gerr))
	})
}
