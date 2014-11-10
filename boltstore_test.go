package session

import (
	"github.com/boltdb/bolt"
	"net/http"
	"testing"
	"time"
)

func Test_BoltStore(t *testing.T) {
	db, err := bolt.Open("test.db", 0644, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		t.Fatalf("bolt error: %s", err)
	}

	store, err := NewBoltStore(db, 60*time.Minute)
	if err != nil {
		t.Errorf("failed to create bolt store: %s", err)
		return
	}

	sm, err := NewSessionManager(store, "test_session")
	if err != nil {
		t.Errorf("failed to create session manager: %s", err)
		return
	}

	memTest := SessionTestServer{t, sm}
	go http.ListenAndServe(listen, memTest)

	doneChan := make(chan bool)
	go func() {
		sessionTest(t)
		doneChan <- true
	}()

	timeout := time.After(5 * time.Second)

	select {
	case <-doneChan:
	case <-timeout:
		t.Fatalf("Timeout.")
	}
}
