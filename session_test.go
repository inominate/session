package session

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"testing"
)

type SessionTestServer struct {
	t  *testing.T
	sm *SessionManager
}

func (m SessionTestServer) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	ses, err := m.sm.Begin(w, req)
	if err != nil {
		m.t.Errorf("Failed to begin session: %s", err)
		return
	}
	defer func() {
		err := ses.Commit()
		if err != nil {
			m.t.Errorf("Failed to commit session: %s", err)
		}
	}()
	req.ParseForm()

	if sesVarName := req.FormValue("clear"); sesVarName != "" {
		ses.Clear()
	}

	if sesVarName := req.FormValue("get"); sesVarName != "" {
		m.t.Logf("Get %s", sesVarName)
		val := ses.Get(sesVarName)
		if val != "" {
		val, ok := ses.Get(sesVarName)
		if ok {
			fmt.Fprintf(w, "Got: %s", val)
			m.t.Logf("Got: %s", val)
		} else {
			fmt.Fprintf(w, "NotFound")
			m.t.Logf("NotFound")
		}
		return
	}

	if sesVarName := req.FormValue("put"); sesVarName != "" {
		m.t.Logf("Put %s", sesVarName)
		ses.Set(sesVarName, req.FormValue("value"))
		m.t.Logf("Set %s", req.FormValue("value"))

		w.Write([]byte(""))
		return
	}

}

const listen = "localhost:54987"

func req(t *testing.T, c *http.Client, params string) string {
	resp, err := c.Get(fmt.Sprintf("http://%s/?%s", listen, params))
	if err != nil {
		t.Errorf("failed http GET: %s", err)
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("http GET expected 200 got %d", resp.StatusCode)
	}

	bytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Errorf("read error: %s", err)
		return ""
	}

	return string(bytes)
}

func getSesID(t *testing.T, jar http.CookieJar) string {
	cookieUrl, err := url.Parse(fmt.Sprintf("http://%s/", listen))
	if err != nil {
		t.Errorf("Invalid cookieURL: %s", err)
		return ""
	}
	cookies := jar.Cookies(cookieUrl)
	for _, cookie := range cookies {
		if cookie.Name == "test_session" {
			return cookie.Value
		}
	}

	return ""
}

func session_test(t *testing.T) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Errorf("failed to create cookiejar: %s", err)
		return
	}

	c := &http.Client{Jar: jar}

	var str string
	var sesID, sesID2 string

	// Check for initial empty session ID
	sesID = getSesID(t, c.Jar)
	if sesID != "" {
		t.Errorf("failed Got session id '%s', expected ''", sesID)
	}
	t.Logf("Starting session id: '%s'", sesID)

	// Test for nonexistant session variable
	str = req(t, c, "get=nothing")
	if str != "NotFound" {
		t.Errorf("failed get=nothing reported as %s", str)
	}

	// ensure a session was actually set up
	sesID = getSesID(t, c.Jar)
	if sesID == "" {
		t.Errorf("failed got session id '%s', expected something", sesID)
	}
	t.Logf("Initial session id: '%s'", sesID)

	// test setting a session variable
	str = req(t, c, "put=something&value=buttes")
	if str != "" {
		t.Errorf("failed put=something reported as '%s', expcted ''", str)
	}

	// test retrieving that variable
	str = req(t, c, "get=something")
	if str != "Got: buttes" {
		t.Errorf("failed get=something reported as '%s', expcted 'Got: buttes'", str)
	}

	// ensure our session IDs are stable
	sesID2 = getSesID(t, c.Jar)
	if sesID2 != sesID {
		t.Errorf("failed got session id '%s', expected '%s'", sesID2, sesID)
	}

	// clear the session and make sure it actually got cleared
	str = req(t, c, "clear=true&get=something")
	if str != "NotFound" {
		t.Errorf("failed get=something reported as '%s', expcted 'NotFound", str)
	}

	// make sure we have a new session id
	sesID2 = getSesID(t, c.Jar)
	if sesID2 == sesID {
		t.Errorf("failed Session ID unchanged after clearing, got '%s'.", sesID)
	}
	t.Logf("New session id: '%s'", sesID2)

	t.Logf("All tests completed.")
}
