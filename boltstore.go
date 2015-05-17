package session

import (
	"bytes"
	"encoding/gob"
	"errors"
	"time"

	"github.com/boltdb/bolt"
)

/*
BoltStore is a session storage using bolt.
*/
type BoltStore struct {
	store *bolt.DB

	lastUsedName []byte
	sessionsName []byte

	maxAge time.Duration
}

const TimeStampFormat = "2006-01-02 15:04:05.000"

/*
NewBoltStore returns a BoltStore SessionStorage.
*/
func NewBoltStore(db *bolt.DB, maxAge time.Duration) (*BoltStore, error) {
	var s BoltStore
	if maxAge < 5*time.Minute {
		return nil, errors.New("maxAge duration too short")
	}

	s.store = db
	s.maxAge = maxAge

	s.lastUsedName = []byte("sessionsLastUsed")
	s.sessionsName = []byte("sessions")

	err := db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(s.lastUsedName)
		if err != nil {
			return err
		}
		_, err = tx.CreateBucketIfNotExists(s.sessionsName)
		if err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	return &s, nil
}

/* Interface Functions */

// Close the Store, will also close the bolt db.
func (s *BoltStore) Close() error {
	return s.store.Close()
}

// GC one pass over the BoltStore
func (s *BoltStore) GC() error {
	err := s.store.Update(func(tx *bolt.Tx) error {
		lastUsedBucket := tx.Bucket(s.lastUsedName)
		sessionsBucket := tx.Bucket(s.sessionsName)

		lastUsedBucket.ForEach(func(k, v []byte) error {
			var t time.Time
			err := t.GobDecode(v)
			if err != nil || time.Since(t) > s.maxAge {
				lastUsedBucket.Delete(k)
				sessionsBucket.Delete(k)
			}
			return nil
		})

		return nil
	})

	return err
}

// Get session associated with sid.
func (s *BoltStore) Get(sid string) (*Session, error) {
	var ses Session

	err := s.store.View(func(tx *bolt.Tx) error {
		lastUsedBucket := tx.Bucket(s.lastUsedName)
		sessionsBucket := tx.Bucket(s.sessionsName)

		bsid := []byte(sid)
		lastUsed := lastUsedBucket.Get(bsid)
		if lastUsed == nil {
			return nil
		}

		var t time.Time
		err := t.GobDecode(lastUsed)
		if err != nil || time.Since(t) > s.maxAge {
			return nil
		}

		sesGob := sessionsBucket.Get(bsid)
		if sesGob == nil {
			return nil
		}

		ses.Values, _ = ungobValues(sesGob)
		return nil
	})

	if err != nil {
		return nil, err
	}

	if ses.Values == nil {
		return nil, ErrNotFound
	}

	return &ses, nil
}

// Commit session back to storage.
func (s *BoltStore) Commit(ses *Session) error {
	err := s.store.Update(func(tx *bolt.Tx) error {
		lastUsedBucket := tx.Bucket(s.lastUsedName)
		sessionsBucket := tx.Bucket(s.sessionsName)

		bsid := []byte(ses.sid)

		tb, err := time.Now().GobEncode()
		if err != nil {
			return err
		}
		err = lastUsedBucket.Put(bsid, tb)
		if err != nil {
			return err
		}

		g, err := gobValues(ses.Values)
		if err != nil {
			return err
		}

		err = sessionsBucket.Put(bsid, g)
		if err != nil {
			return err
		}

		return nil
	})

	return err
}

// Convert a map[string]string to a gobbed []byte
func gobValues(v map[string]string) ([]byte, error) {
	b := &bytes.Buffer{}
	g := gob.NewEncoder(b)

	err := g.Encode(v)
	return b.Bytes(), err
}

// Convert a gobbed map[string]string back to a map.
func ungobValues(v []byte) (map[string]string, error) {
	b := bytes.NewBuffer(v)
	g := gob.NewDecoder(b)

	var values map[string]string
	err := g.Decode(&values)
	return values, err
}

// Delete session from storage.
func (s *BoltStore) Delete(ses *Session) error {
	err := s.store.Update(func(tx *bolt.Tx) error {
		lastUsedBucket := tx.Bucket(s.lastUsedName)
		sessionsBucket := tx.Bucket(s.sessionsName)

		err := lastUsedBucket.Delete([]byte(ses.sid))
		if err != nil {
			return err
		}

		err = sessionsBucket.Delete([]byte(ses.sid))
		if err != nil {
			return err
		}

		return nil
	})

	return err
}
