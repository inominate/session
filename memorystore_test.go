package session

import (
	"net/http"
	"testing"
	"time"
)

func Test_MemoryStore(t *testing.T) {
	store, err := NewMemoryStore(60 * time.Minute)
	if err != nil {
		t.Errorf("failed to create memory store: %s", err)
		return
	}

	sm, err := NewSessionManager(store, "test_session")
	if err != nil {
		t.Errorf("failed to create session manager: %s", err)
		return
	}

	memTest := SessionTestServer{t, sm}
	go http.ListenAndServe(listen, memTest)

	sessionTest(t)
}
