package director

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/shopmonkeyus/go-common/logger"
	"github.com/stretchr/testify/assert"
)

type mockDirector struct {
	body string
	auth string
	conn net.Listener
	port int
}

func (d *mockDirector) reset() {
	d.body = ""
	d.auth = ""
}

func (d *mockDirector) Close() error {
	return d.conn.Close()
}

func (d *mockDirector) handle(w http.ResponseWriter, req *http.Request) {
	if req.Method != "POST" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	buf, err := io.ReadAll(req.Body)
	if err != nil {
		w.WriteHeader(500)
		return
	}
	d.body = string(buf)
	d.auth = req.Header.Get("Authorization")
	w.WriteHeader(http.StatusAccepted)
}

func NewMockDirector() (*mockDirector, error) {
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		return nil, fmt.Errorf("listen: %w", err)
	}
	addr := ln.Addr().String()
	i := strings.LastIndex(addr, ":")
	port, err := strconv.ParseInt(addr[i+1:], 10, 32)
	if err != nil {
		return nil, fmt.Errorf("port: %s %w", addr, err)
	}
	mux := http.NewServeMux()
	director := &mockDirector{
		conn: ln,
		port: int(port),
	}
	mux.HandleFunc("/--/register", director.handle)
	go http.Serve(ln, mux)
	return director, nil
}

var testTime = time.Date(2023, 10, 22, 12, 30, 00, 0, time.UTC)

func TestRegistration(t *testing.T) {
	director, err := NewMockDirector()
	assert.NoError(t, err)
	assert.Greater(t, director.port, 0)
	defer director.Close()
	reg, err := NewRegistration(logger.NewTestLogger(), "localhost", "127.0.0.1", 8989,
		WithURL(fmt.Sprintf("http://localhost:%d", director.port)),
		withTimestamp(testTime),
		WithInterval(time.Second),
		WithAuthorization("1234"),
		WithRegion(""), // test that it's not set if empty
	)
	assert.NoError(t, err)
	assert.Equal(t, "1234", director.auth)
	assert.Equal(t, `{"timestamp":"2023-10-22T12:30:00Z","status":"UP","region":"dev","ipAddress":"127.0.0.1","port":8989,"hostname":"localhost"}`, director.body)
	director.reset()
	time.Sleep(time.Second * 2)
	assert.Equal(t, `{"timestamp":"2023-10-22T12:30:00Z","status":"UP","region":"dev","ipAddress":"127.0.0.1","port":8989,"hostname":"localhost"}`, director.body)
	director.reset()
	reg.Close()
	assert.Equal(t, `{"timestamp":"2023-10-22T12:30:00Z","status":"DOWN","region":"dev","ipAddress":"127.0.0.1","port":8989,"hostname":"localhost"}`, director.body)
}
