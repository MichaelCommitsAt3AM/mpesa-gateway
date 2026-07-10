package server

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/mpesa-gateway/internal/config"
	"github.com/mpesa-gateway/internal/handlers"
	"github.com/mpesa-gateway/internal/tenant"
	"github.com/stretchr/testify/require"
)

// newTestServer builds a Server with nil-backed dependencies. Safe here
// because these tests only exercise a route registered directly via
// Router(), never /health, /initiate, or /callback, which are the only
// handlers that would dereference the db/paymentService/queueClient/tenant
// store.
func newTestServer(t *testing.T) *Server {
	t.Helper()
	cfg := &config.Config{ServerPort: "0"}
	h := handlers.NewHandler(nil, nil, nil)
	store := tenant.NewStore(nil)
	return NewServer(cfg, h, store)
}

// TestServer_GracefulShutdownDrainsInFlightRequests proves the fix for the
// "no graceful HTTP shutdown" blocker: a request already in flight when
// Shutdown is called must be allowed to finish, not cut off, and Shutdown
// must return once it has drained rather than hanging or timing out.
func TestServer_GracefulShutdownDrainsInFlightRequests(t *testing.T) {
	s := newTestServer(t)

	started := make(chan struct{})
	release := make(chan struct{})
	s.Router().Get("/slow", func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		w.WriteHeader(http.StatusOK)
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- s.Serve(ln)
	}()

	type result struct {
		status int
		err    error
	}
	reqDone := make(chan result, 1)
	go func() {
		resp, err := http.Get("http://" + ln.Addr().String() + "/slow")
		if err != nil {
			reqDone <- result{err: err}
			return
		}
		defer resp.Body.Close()
		reqDone <- result{status: resp.StatusCode}
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("in-flight request never reached the handler")
	}

	shutdownDone := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		shutdownDone <- s.Shutdown(ctx)
	}()

	// Give Shutdown a moment to start draining before releasing the
	// in-flight handler, to prove it waits rather than cutting the
	// connection out from under the request.
	time.Sleep(100 * time.Millisecond)
	close(release)

	res := <-reqDone
	require.NoError(t, res.err, "in-flight request must complete successfully, not be cut off by shutdown")
	require.Equal(t, http.StatusOK, res.status)

	require.NoError(t, <-shutdownDone, "Shutdown should return cleanly, not via its timeout")
	require.ErrorIs(t, <-serveErr, http.ErrServerClosed)
}
