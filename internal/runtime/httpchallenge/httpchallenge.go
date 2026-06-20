package httpchallenge

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const challengePathPrefix = "/.well-known/acme-challenge/"

type Server struct {
	addr       string
	challenges map[string]string
	mu         sync.RWMutex
	server     *http.Server
	done       chan struct{}
	started    bool
	closed     bool
}

var ErrServerClosed = errors.New("httpchallenge server cannot be restarted after shutdown")

func New(addr string) *Server {
	if strings.TrimSpace(addr) == "" {
		addr = ":80"
	}
	s := &Server{addr: addr, challenges: map[string]string{}, done: make(chan struct{})}
	s.server = &http.Server{Handler: http.HandlerFunc(s.handle)}
	return s
}

func (s *Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrServerClosed
	}
	if s.started {
		return nil
	}
	listener, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	s.started = true
	go func() {
		_ = s.server.Serve(listener)
		close(s.done)
	}()
	return nil
}

func (s *Server) Set(token, response string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.challenges[token] = response
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.RLock()
	started := s.started
	s.mu.RUnlock()
	if !started {
		return nil
	}
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
	}
	err := s.server.Shutdown(ctx)
	select {
	case <-s.done:
	case <-ctx.Done():
		return ctx.Err()
	}
	s.mu.Lock()
	s.started = false
	s.closed = true
	s.mu.Unlock()
	return err
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.URL.Path, challengePathPrefix) {
		http.NotFound(w, r)
		return
	}
	token := strings.TrimPrefix(r.URL.Path, challengePathPrefix)
	if token == "" {
		http.NotFound(w, r)
		return
	}
	s.mu.RLock()
	response, ok := s.challenges[token]
	s.mu.RUnlock()
	if ok {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(response))
		return
	}
	http.NotFound(w, r)
}
