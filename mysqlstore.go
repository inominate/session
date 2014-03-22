/* Sessions Package */
package session

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

type MySQLStore struct {
	db                *sql.DB
	startSessionStmt  *sql.Stmt
	commitSessionStmt *sql.Stmt
	gcSessionStmt     *sql.Stmt
	delSessionStmt    *sql.Stmt
}

/*
	Create a new session storage in the given mysql database
*/
func NewMySQLStore(db *sql.DB, tablename string, maxAge time.Duration) (*MySQLStore, error) {
	var s MySQLStore

	if tablename == "" {
		return nil, fmt.Errorf("can not use empty table name")
	}

	s.db = db

	_, err := db.Query(fmt.Sprintf("CREATE TABLE IF NOT EXISTS `%s` (", tablename) +
		" `sid` char(40) NOT NULL," +
		" `atime` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP," +
		" `data` text NOT NULL," +
		" PRIMARY KEY (`sid`)," +
		" KEY `atime` (`atime`)" +
		" ) ENGINE=MyISAM DEFAULT CHARSET=utf8")
	if err != nil {
		return nil, fmt.Errorf("failed attempting to create table: %s", err)
	}

	s.startSessionStmt, err = db.Prepare(fmt.Sprintf("select data from sessions where sid = ? and subdate(now(), interval %d second) < atime", int(maxAge.Seconds())))
	if err != nil {
		return nil, fmt.Errorf("failed preparing startSessionStmt: %s", err)
	}
	s.commitSessionStmt, err = db.Prepare("replace into sessions (sid, data) VALUES (?, ?)")
	if err != nil {
		return nil, fmt.Errorf("failed preparing commitSessionStmt: %s", err)
	}
	s.gcSessionStmt, err = db.Prepare(fmt.Sprintf("delete from sessions where subdate(now(), interval %d second) > atime", int(maxAge.Seconds())))
	if err != nil {
		return nil, fmt.Errorf("failed preparing gcSessionStmt: %s", err)
	}
	s.delSessionStmt, err = db.Prepare("delete from sessions where sid = ?")
	if err != nil {
		return nil, fmt.Errorf("failed preparing delSessionStmt: %s", err)
	}

	return &s, nil
}

func (s *MySQLStore) Close() error {
	err1 := s.startSessionStmt.Close()
	err2 := s.commitSessionStmt.Close()
	err3 := s.gcSessionStmt.Close()
	err4 := s.delSessionStmt.Close()

	if err1 != nil {
		return fmt.Errorf("error closing startSessionStmt: %s", err1)
	}
	if err2 != nil {
		return fmt.Errorf("error closing commitSessionStmt: %s", err2)
	}
	if err3 != nil {
		return fmt.Errorf("error closing gcSessionStmt: %s", err3)
	}
	if err4 != nil {
		return fmt.Errorf("error closing delSessionStmt: %s", err4)
	}
	return nil
}

func (s *MySQLStore) GC() error {
	_, err := s.gcSessionStmt.Exec()
	return err
}

func (s *MySQLStore) Get(sid string) (*Session, error) {
	var ses Session

	var session_json []byte
	err := s.startSessionStmt.QueryRow(sid).Scan(&session_json)
	if err == nil {
		ses.sid = sid
		json.Unmarshal(session_json, &ses.Values)
		return &ses, nil
	}
	if err != sql.ErrNoRows {
		return nil, err
	}
	return nil, ErrNotFound
}

func (s *MySQLStore) Commit(ses *Session) error {
	if ses.sid != "" {
		session_json, err := json.Marshal(ses.Values)
		if err != nil {
			return err
		}
		_, err = s.commitSessionStmt.Exec(ses.sid, session_json)
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *MySQLStore) Delete(ses *Session) error {
	_, err := s.delSessionStmt.Exec(ses.sid)
	return err
}
