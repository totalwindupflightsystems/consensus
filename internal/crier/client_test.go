package crier

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"testing"
	"time"
)

// --- registration --------------------------------------------------------

// TestClient_RegisterToleratesDuplicate covers the registry's one refusal that
// is not a failure for an intake caller: a duplicate id answers 409 with the
// existing row untouched, which means the identity the caller needs already
// exists.
func TestClient_RegisterToleratesDuplicate(t *testing.T) {
	srv := newFakeCrier().start(t)
	client := NewClient(srv.URL)
	ctx := context.Background()

	priv, _ := newKeypair(t)
	first, err := client.Register(ctx, RegisterInput{ID: "consensus", PublicKey: PublicKeyHex(priv), Capabilities: []string{"inbox"}})
	if err != nil {
		t.Fatalf("first register: %v", err)
	}
	if first.ID != "consensus" || first.Status != "online" {
		t.Errorf("registered agent = %+v, want id consensus and status online", first)
	}

	second, err := client.Register(ctx, RegisterInput{ID: "consensus", PublicKey: PublicKeyHex(priv)})
	if err != nil {
		t.Fatalf("duplicate register must be tolerated, got: %v", err)
	}
	if second.ID != "consensus" {
		t.Errorf("duplicate register id = %q, want consensus", second.ID)
	}
}

// --- deliver / retrieve round trip ---------------------------------------

// TestClient_DeliverAndRetrieveRoundTrip proves the payload survives the wire
// byte for byte: deliver takes a JSON object, crier stores raw bytes and
// returns base64, and the client decodes them back.
func TestClient_DeliverAndRetrieveRoundTrip(t *testing.T) {
	fake := newFakeCrier()
	srv := fake.start(t)
	client := NewClient(srv.URL)
	ctx := context.Background()

	payload := map[string]any{"text": "hello from a producer", "topic": "consensus.inbox", "n": float64(42)}
	if _, err := client.Register(ctx, RegisterInput{ID: "consensus"}); err != nil {
		t.Fatalf("register consumer: %v", err)
	}
	accepted, err := client.Deliver(ctx, "consensus", DeliverInput{Payload: payload, Sender: "producer-1"})
	if err != nil {
		t.Fatalf("deliver: %v", err)
	}
	if accepted.ID == "" || accepted.Transport != "inbox" {
		t.Fatalf("deliver result = %+v, want a message id and transport inbox", accepted)
	}

	leased, err := client.Retrieve(ctx, "consensus", RetrieveOptions{})
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	if len(leased.Messages) != 1 {
		t.Fatalf("retrieved %d messages, want 1", len(leased.Messages))
	}
	entry := leased.Messages[0]
	if entry.ID != accepted.ID {
		t.Errorf("retrieved id = %q, want the delivered id %q", entry.ID, accepted.ID)
	}
	if entry.Sender != "producer-1" {
		t.Errorf("retrieved sender = %q, want producer-1 (crier records the sender with the message)", entry.Sender)
	}
	if got := decodeObject(t, entry.Payload); !reflect.DeepEqual(got, payload) {
		t.Errorf("payload round trip = %v, want the delivered object %v verbatim", got, payload)
	}
	if leased.LeaseID == "" {
		t.Error("lease_id is empty after claiming a message; nothing could be acked")
	}

	if err := client.Ack(ctx, "consensus", leased.LeaseID, []string{entry.ID}); err != nil {
		t.Fatalf("ack: %v", err)
	}
	if ids := fake.pending("consensus"); len(ids) != 0 {
		t.Errorf("inbox still holds %v after ack, want drained", ids)
	}
}

// TestClient_DeliverRejectsEmptyPayload mirrors crier's own validation: a
// delivery with no payload is refused server-side, so the client refuses to
// send one instead of burning a round trip.
func TestClient_DeliverRejectsEmptyPayload(t *testing.T) {
	fake := newFakeCrier()
	srv := fake.start(t)
	client := NewClient(srv.URL)
	if _, err := client.Deliver(context.Background(), "consensus", DeliverInput{}); err == nil {
		t.Fatal("deliver with no payload must fail")
	}
	if ids := fake.pending("consensus"); len(ids) != 0 {
		t.Errorf("inbox holds %v after a refused delivery, want nothing stored", ids)
	}
}

// --- empty inbox ---------------------------------------------------------

// TestClient_RetrieveEmptyInboxIsAnEmptyArray pins crier's documented empty
// shape: a 200 with messages [] and an empty lease_id — a successful read with
// nothing to claim, never null and never an error.
func TestClient_RetrieveEmptyInboxIsAnEmptyArray(t *testing.T) {
	fake := newFakeCrier()
	fake.register("consensus")
	srv := fake.start(t)

	leased, err := NewClient(srv.URL).Retrieve(context.Background(), "consensus", RetrieveOptions{})
	if err != nil {
		t.Fatalf("retrieve on an empty inbox: %v", err)
	}
	if leased.Messages == nil {
		t.Error("messages is nil, want an empty array (crier always sends [])")
	}
	if len(leased.Messages) != 0 {
		t.Errorf("retrieved %d messages from an empty inbox", len(leased.Messages))
	}
	if leased.LeaseID != "" {
		t.Errorf("lease_id = %q on an empty read, want empty (nothing to ack)", leased.LeaseID)
	}
}

// --- signing -------------------------------------------------------------

// TestClient_RetrieveSignsWithTheAgentKey proves the wire contract itself: the
// headers crier verifies are present AND the signature verifies against the
// agent's public key over "METHOD\n<path>\n<timestamp>", with the timestamp
// inside the ±30s window.
func TestClient_RetrieveSignsWithTheAgentKey(t *testing.T) {
	var captured *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"messages":[],"lease_id":"","queue_depth":0,"leased_count":0}`))
	}))
	defer srv.Close()

	priv, pub := newKeypair(t)
	client := NewClient(srv.URL).WithSigner("consensus", NewEd25519Signer(priv))

	if _, err := client.Retrieve(context.Background(), "consensus", RetrieveOptions{Limit: 5, LeaseSeconds: 30, WaitSeconds: 0}); err != nil {
		t.Fatalf("signed retrieve: %v", err)
	}
	if captured == nil {
		t.Fatal("the server never saw a request")
	}

	gotID := captured.Header.Get(headerAgentID)
	gotTs := captured.Header.Get(headerAgentTs)
	gotSig := captured.Header.Get(headerAgentSig)
	if gotID != "consensus" || gotTs == "" || gotSig == "" {
		t.Fatalf("signature headers = id %q ts %q sig %q, want the full triple", gotID, gotTs, gotSig)
	}

	ts, err := strconv.ParseInt(gotTs, 10, 64)
	if err != nil {
		t.Fatalf("X-Agent-Ts %q is not a unix timestamp: %v", gotTs, err)
	}
	if delta := time.Now().Unix() - ts; delta > 30 || delta < -30 {
		t.Errorf("X-Agent-Ts is %ds from now, want within ±30s", delta)
	}

	sig, err := hex.DecodeString(gotSig)
	if err != nil {
		t.Fatalf("X-Agent-Sig %q is not hex: %v", gotSig, err)
	}
	// The query string is not part of the signed payload (crier §3), so the
	// path alone is what must verify.
	signed := SignaturePayload(http.MethodGet, "/agents/consensus/inbox", ts)
	if !ed25519.Verify(pub, []byte(signed), sig) {
		t.Errorf("signature does not verify over %q — the client signs the wrong payload", signed)
	}

	// Control: the same signature must NOT verify over the path WITH the query
	// string, which is the mistake this test exists to catch.
	withQuery := SignaturePayload(http.MethodGet, captured.URL.RequestURI(), ts)
	if withQuery != signed && ed25519.Verify(pub, []byte(withQuery), sig) {
		t.Error("signature also verifies over the query string; the non-vacuity control is broken")
	}
}

// TestClient_UnsignedRetrieveRefusedWhenSignaturesRequired is the 401 half of
// the auth contract: against a signature-enforcing relay (crier's default) an
// unsigned agent-scoped read is refused, and the refusal surfaces as an
// APIError carrying the status.
func TestClient_UnsignedRetrieveRefusedWhenSignaturesRequired(t *testing.T) {
	fake := newFakeCrier()
	fake.requireSig = true
	fake.register("consensus")
	srv := fake.start(t)

	_, err := NewClient(srv.URL).Retrieve(context.Background(), "consensus", RetrieveOptions{})
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("unsigned retrieve error = %v, want *APIError", err)
	}
	if apiErr.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", apiErr.StatusCode)
	}
}

// TestClient_CrossAgentRetrieveForbidden is the identity half: a validly
// signed request presented as another agent is refused 403, so a signature can
// never be borrowed to read someone else's inbox.
func TestClient_CrossAgentRetrieveForbidden(t *testing.T) {
	fake := newFakeCrier()
	fake.requireSig = true
	fake.register("consensus")
	fake.register("consensus-b")
	srv := fake.start(t)

	priv, _ := newKeypair(t)
	// Signed as consensus-b, retrieving consensus's inbox.
	client := NewClient(srv.URL).WithSigner("consensus-b", NewEd25519Signer(priv))

	_, err := client.Retrieve(context.Background(), "consensus", RetrieveOptions{})
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("cross-agent retrieve error = %v, want *APIError", err)
	}
	if apiErr.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403 (the caller may only access its own resources)", apiErr.StatusCode)
	}
}

// --- helpers -------------------------------------------------------------

// newKeypair returns an ed25519 keypair for the signing tests.
func newKeypair(t *testing.T) (ed25519.PrivateKey, ed25519.PublicKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate ed25519 key: %v", err)
	}
	return priv, pub
}

// decodeObject parses a JSON object payload for a semantic comparison (so the
// assertion is about the round trip, not about key ordering or spacing).
func decodeObject(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("payload %s is not a JSON object: %v", raw, err)
	}
	return out
}
