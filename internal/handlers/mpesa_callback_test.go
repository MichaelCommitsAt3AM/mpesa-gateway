package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/require"
)

// newTestQueueClient backs an asynq.Client with an in-memory miniredis
// instance, so callback tests can verify enqueue behavior without a real
// Redis dependency.
func newTestQueueClient(t *testing.T) *asynq.Client {
	t.Helper()

	mr := miniredis.RunT(t)
	client := asynq.NewClient(asynq.RedisClientOpt{Addr: mr.Addr()})
	t.Cleanup(func() { client.Close() })

	return client
}

const validCallbackPayload = `{
	"Body": {
		"stkCallback": {
			"MerchantRequestID": "29115-34620561-1",
			"CheckoutRequestID": "ws_CO_11012024135500",
			"ResultCode": 0,
			"ResultDesc": "The service request is processed successfully."
		}
	}
}`

func TestMPesaCallback_ValidPayloadIsQueued(t *testing.T) {
	h := &Handler{queueClient: newTestQueueClient(t)}

	req := httptest.NewRequest(http.MethodPost, "/callback", strings.NewReader(validCallbackPayload))
	rec := httptest.NewRecorder()

	h.MPesaCallback(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{"status":"received"}`, rec.Body.String())
}

func TestMPesaCallback_MalformedJSONRejected(t *testing.T) {
	h := &Handler{queueClient: newTestQueueClient(t)}

	req := httptest.NewRequest(http.MethodPost, "/callback", strings.NewReader(`{not valid json`))
	rec := httptest.NewRecorder()

	h.MPesaCallback(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestMPesaCallback_EmptyBodyRejected(t *testing.T) {
	h := &Handler{queueClient: newTestQueueClient(t)}

	req := httptest.NewRequest(http.MethodPost, "/callback", strings.NewReader(``))
	rec := httptest.NewRecorder()

	h.MPesaCallback(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}
