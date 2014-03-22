/*
Package session implements a simple session handler for use with the Go http
package.
*/
package session

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

//	SessionManager type, use NewSessionManager() to create.
type SessionManager struct {
	// Set true to require Secure cookies
	Secure bool

	gcDelay   time.Duration
	closeChan chan bool

	cookieName string

	storage SessionStorage
	sync.RWMutex

	closed bool
}

/*
SessionStorage interface is used and required by SessionManager.

Sessions passed as parameters can be used concurrently. All methods except
Close() must be able to function concurrently.
*/
type SessionStorage interface {
	/*
		Return a session associated with sid. Only the Values map is expected
		to exist. Return ErrNotFound if no associated session with sid is found.
	*/
	Get(sid string) (*Session, error)

	/*
		Commit a session back into storage
	*/
	Commit(session *Session) error

	/*
		Delete a session from storage. NOP if the session isn't in storage,
		only returns an error if something goes seriously wrong.
	*/
	Delete(session *Session) error

	/*
		Will be called periodically(see SetGCDelay()) to clean up expired
		sessions
	*/
	GC() error

	/*
		Close the session storage peforming whatever cleanup is necessary.
	*/
	Close() error
}

/*
SessionStorage implementations should return ErrNotFound when Get() finds no
associated session.
*/
var ErrNotFound = errors.New("no session found")

/*
Session may be used concurrently, but should only be used in conjunction with a
single HTTP request.
*/
type Session struct {
	sid        string
	req        *http.Request
	w          http.ResponseWriter
	cookieName string
	secure     bool

	storage SessionStorage
	sync.RWMutex

	// Available for external use at your own risk.
	Values map[string]string
}

/*
NewSessionManager will initialize the sessions system. Expects a previously
created SessionStorage and the name of the http cookie to use.

Once created, SessionManager.Secure can be set to force secure cookies.
*/
func NewSessionManager(storage SessionStorage, cookieName string) (*SessionManager, error) {
	var sm SessionManager

	if cookieName == "" {
		return nil, errors.New("invalid cookie Name")
	}

	sm.gcDelay = time.Hour
	sm.cookieName = cookieName

	sm.storage = storage
	sm.closeChan = make(chan bool)

	go sm.gc()

	return &sm, nil
}

/*
Close the session manager, ending the gc loop and doing whatever cleanup the
storage manager demands.
*/
func (sm *SessionManager) Close() error {
	sm.Lock()
	defer sm.Unlock()

	if sm.closed {
		return errors.New("already closed")
	}

	var gcErr error

	select {
	case sm.closeChan <- true:
		close(sm.closeChan)
	case <-time.After(30 * time.Second):
		gcErr = errors.New("gc failed to shut down")

		// If we do time out, let's make sure that if something ever does come
		// back we handle it.
		go func() {
			<-sm.closeChan
			close(sm.closeChan)
		}()
	}
	sm.closed = true

	err := sm.storage.Close()
	if err != nil {
		return err
	}

	if gcErr != nil {
		return gcErr
	}

	return nil
}

/*
	SetGCDelay is used to configure time between purging expired sessions.
	Default is every hour.
*/
func (sm *SessionManager) SetGCDelay(delay time.Duration) error {
	sm.Lock()
	defer sm.Unlock()

	if delay < 5*time.Minute {
		return errors.New("maxAge duration too short")
	}

	sm.gcDelay = delay
	return nil
}

func (sm *SessionManager) gc() {
	for {
		select {
		case <-sm.closeChan:
			return
		case <-time.After(sm.gcDelay):
			sm.Lock()
			err := sm.storage.GC()
			sm.Unlock()
			if err != nil {
				panic(err)
			}
		}
	}
}

/*
Begin using a session. Returns a session, resuming an existing session if
possible and creating a	new session if necessary.
*/
func (sm *SessionManager) Begin(w http.ResponseWriter, req *http.Request) (*Session, error) {
	var s Session

	sidCookie, err := req.Cookie(sm.cookieName)
	if err == nil && sidCookie.Value != "" {
		s.sid = sidCookie.Value

		stored, err := sm.storage.Get(s.sid)
		if err != nil && err != ErrNotFound {
			return nil, err
		}
		if stored != nil {
			s.Values = stored.Values
		}
	}

	s.storage = sm.storage
	s.cookieName = sm.cookieName
	s.secure = sm.Secure

	s.req = req
	s.w = w

	if s.Values == nil {
		s.Clear()
	} else {
		s.setCookie()
	}
	return &s, nil
}

/*
ActionToken will return a token which can be embedded into forms to prevent
cross site request attacks.
*/
func (s *Session) ActionToken() string {
	sat, ok := s.Get("actionToken")
	if ok {
		return sat
	}
	return "error"
}

/*
Checks the current action token against the token in the request. Expects a
form value named "actionToken". Returns true if it's a real request.
*/
func (s *Session) CanAct() bool {
	at := s.req.FormValue("actionToken")
	sat, ok := s.Get("actionToken")
	if ok && at != "error" && at == sat {
		return true
	}
	return false
}

/*
NewActionToken resets the action token, should be used after each checked
action is performed.
*/
func (s *Session) NewActionToken() string {
	s.Set("actionToken", makeID())
	return s.ActionToken()
}

/*
Gets a session variable.  Returns "", false if session variable is not set.
*/
func (s *Session) Get(key string) (string, bool) {
	s.RLock()
	defer s.RUnlock()

	val, ok := s.Values[key]
	return val, ok
}

/*
Set a session variable.
*/
func (s *Session) Set(key string, value string) {
	s.Lock()
	defer s.Unlock()

	s.Values[key] = value
}

/*
Commit the session back to storage. Should be called at the end of each
request.
*/
func (s *Session) Commit() error {
	s.Lock()
	defer s.Unlock()

	if s.sid != "" {
		return s.storage.Commit(s)
	}

	return nil
}

func (s *Session) setCookie() {
	var sessionCookie http.Cookie

	sessionCookie.Name = s.cookieName
	sessionCookie.Value = s.sid
	sessionCookie.Path = "/"
	sessionCookie.MaxAge = 86400 * 30
	sessionCookie.HttpOnly = true
	if s.secure {
		sessionCookie.Secure = true
	}

	http.SetCookie(s.w, &sessionCookie)
}

/*
Clear existing session data leaving a new one.
*/
func (s *Session) Clear() {
	s.Lock()

	s.storage.Delete(s)

	s.sid = makeID()
	s.Values = make(map[string]string)
	s.Unlock()

	s.setCookie()
	s.NewActionToken()
}

func makeID() string {
	buf := make([]byte, 32)
	io.ReadFull(rand.Reader, buf)
	return fmt.Sprintf("%x", buf)
}
