package session

import (
	"errors"
	"time"
)

type storedSession struct {
	sid      string
	lastUsed time.Time
	values   map[string]string
}

/*
MemoryStore is a session storage that operates entirely in memory suitable
for testing and small scale uses.
*/
type MemoryStore struct {
	commitQueue chan request
	deleteQueue chan request
	gcQueue     chan request
	getQueue    chan request

	closeChan chan request

	store map[string]storedSession

	maxAge time.Duration
}

type request struct {
	sid     string
	session *Session
	err     error

	respChan chan request
}

/*
NewMemoryStore returns a MemoryStore SessionStorage.
*/
func NewMemoryStore(maxAge time.Duration) (*MemoryStore, error) {
	var s MemoryStore
	if maxAge < 5*time.Minute {
		return nil, errors.New("maxAge duration too short")
	}

	s.getQueue = make(chan request, 10)
	s.commitQueue = make(chan request, 10)
	s.gcQueue = make(chan request)
	s.deleteQueue = make(chan request, 10)
	s.closeChan = make(chan request)

	s.store = make(map[string]storedSession)

	s.maxAge = maxAge

	go s.serve()
	return &s, nil
}

/* Interface Functions */

// Close the MemoryStore
func (s *MemoryStore) Close() error {
	respChan := make(chan request)
	req := request{respChan: respChan}

	s.closeChan <- req
	resp := <-respChan

	close(respChan)
	return resp.err
}

// GC one pass over the MemoryStore
func (s *MemoryStore) GC() error {
	respChan := make(chan request)
	req := request{respChan: respChan}

	s.gcQueue <- req
	resp := <-respChan

	close(respChan)
	return resp.err
}

// Get session associated with sid.
func (s *MemoryStore) Get(sid string) (*Session, error) {
	respChan := make(chan request)
	req := request{sid: sid, respChan: respChan}

	s.getQueue <- req
	resp := <-respChan

	close(respChan)
	return resp.session, resp.err
}

// Commit session back to storage.
func (s *MemoryStore) Commit(ses *Session) error {
	respChan := make(chan request)
	req := request{session: ses, respChan: respChan}

	s.commitQueue <- req
	resp := <-respChan

	close(respChan)
	return resp.err
}

// Delete session from storage.
func (s *MemoryStore) Delete(ses *Session) error {
	respChan := make(chan request)
	req := request{session: ses, respChan: respChan}

	s.deleteQueue <- req
	resp := <-respChan

	close(respChan)
	return resp.err
}

// serve acts as the main loop for handling storage operations.
func (s *MemoryStore) serve() {
	for {
		select {
		case req := <-s.commitQueue:
			req.err = s.commit(req.session)
			req.respChan <- req

		case req := <-s.deleteQueue:
			req.err = s.delete(req.session)
			req.respChan <- req

		case req := <-s.gcQueue:
			req.err = s.gc()
			req.respChan <- req

		case req := <-s.getQueue:
			req.session, req.err = s.get(req.sid)
			req.respChan <- req

		case req := <-s.closeChan:
			req.err = s.close()
			req.respChan <- req
			return
		}
	}
}

/* Below are the real work functions, should never be called externally. */
func (s *MemoryStore) close() error {
	close(s.commitQueue)
	close(s.deleteQueue)
	close(s.gcQueue)
	close(s.getQueue)
	close(s.closeChan)

	s.store = nil

	return nil
}

func (s *MemoryStore) gc() error {
	for k := range s.store {
		if time.Since(s.store[k].lastUsed) > s.maxAge {
			delete(s.store, k)
		}
	}

	return nil
}

func (s *MemoryStore) get(sid string) (*Session, error) {
	stored, ok := s.store[sid]
	if !ok {
		return nil, ErrNotFound
	}

	var ses Session
	ses.Values = copyValues(stored.values)

	return &ses, nil
}

func (s *MemoryStore) commit(ses *Session) error {
	store := storedSession{
		sid:      ses.sid,
		lastUsed: time.Now(),
		values:   copyValues(ses.Values),
	}
	s.store[ses.sid] = store

	return nil
}

func (s *MemoryStore) delete(ses *Session) error {
	delete(s.store, ses.sid)

	return nil
}

func copyValues(src map[string]string) map[string]string {
	newMap := make(map[string]string, len(src))

	for k, v := range src {
		newMap[k] = v
	}

	return newMap
}
