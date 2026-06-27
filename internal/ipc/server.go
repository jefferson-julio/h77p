package ipc

import (
	"encoding/json"
	"net"
	"os"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
)

// Msg is injected into the BubbleTea update loop when a Command arrives from
// the client. The root Model routes it to the active sub-model.
type Msg struct{ Cmd Command }

// Sender is satisfied by *tea.Program.
type Sender interface {
	Send(tea.Msg)
}

// Server listens on a Unix domain socket and mediates IPC with one client at
// a time. A new connection atomically replaces the previous one.
type Server struct {
	path     string
	listener net.Listener

	mu   sync.Mutex
	conn net.Conn
	enc  *json.Encoder

	progMu sync.RWMutex
	prog   Sender
}

// New creates a Unix domain socket at socketPath and starts the accept
// goroutine. Call SetProgram with the *tea.Program before the first client
// connects so commands can be forwarded into the update loop.
func New(socketPath string) (*Server, error) {
	// Remove a stale socket left by a previous run.
	_ = os.Remove(socketPath)
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, err
	}
	s := &Server{path: socketPath, listener: ln}
	go s.acceptLoop()
	return s, nil
}

// SetProgram registers the BubbleTea program so that incoming commands are
// forwarded via p.Send. Must be called before p.Run().
func (s *Server) SetProgram(prog Sender) {
	s.progMu.Lock()
	s.prog = prog
	s.progMu.Unlock()
}

// Send writes an Event to the currently connected client. Safe to call from
// any goroutine. Returns nil silently when no client is connected.
func (s *Server) Send(e Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.enc == nil {
		return nil
	}
	return s.enc.Encode(e)
}

// Connected reports whether a client is currently connected.
func (s *Server) Connected() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.conn != nil
}

// Close shuts down the listener and removes the socket file.
func (s *Server) Close() {
	s.listener.Close()
	_ = os.Remove(s.path)
}

func (s *Server) acceptLoop() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return // listener was closed
		}
		s.mu.Lock()
		if s.conn != nil {
			s.conn.Close()
		}
		s.conn = conn
		s.enc = json.NewEncoder(conn)
		s.mu.Unlock()
		go s.readLoop(conn)
	}
}

func (s *Server) readLoop(conn net.Conn) {
	dec := json.NewDecoder(conn)
	for {
		var cmd Command
		if err := dec.Decode(&cmd); err != nil {
			break
		}
		s.progMu.RLock()
		prog := s.prog
		s.progMu.RUnlock()
		if prog != nil {
			prog.Send(tea.Msg(Msg{Cmd: cmd}))
		}
	}
	s.mu.Lock()
	if s.conn == conn {
		s.conn = nil
		s.enc = nil
	}
	s.mu.Unlock()
	conn.Close()
}
