package hookd

import (
	"fmt"
	"net"
	"net/rpc"
	"path/filepath"
	"sync"
)

// Request identifies one command in an active hook context.
type Request struct {
	ContextID   string
	Dir         string
	CommandName string
	Args        []string
}

// Response carries command output and its exit status back to the hook tool.
type Response struct {
	Code   int
	Stdout []byte
	Stderr []byte
}

// Handler validates requests and serializes tool commands within a hook.
type Handler struct {
	mu      sync.Mutex
	context Context
}

// NewHandler binds an RPC command handler to a single hook invocation.
func NewHandler(ctx Context) *Handler { return &Handler{context: ctx} }

// Main validates hook identity and serializes command execution within the invocation.
func (h *Handler) Main(req Request, resp *Response) error {
	if req.ContextID != h.context.ID() {
		return fmt.Errorf("wrong hook context ID")
	}
	if !filepath.IsAbs(req.Dir) {
		return fmt.Errorf("hook tool directory must be absolute")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	command, err := NewCommand(h.context, req.CommandName)
	if err != nil {
		return err
	}
	output, err := command.Run(req.Args)
	if err != nil {
		resp.Code = 1
		resp.Stderr = []byte(err.Error() + "\n")
		return nil
	}
	resp.Stdout = output
	return nil
}

// Server owns the socket, accept loop, and all active RPC connections for a hook.
type Server struct {
	listener    net.Listener
	rpc         *rpc.Server
	mu          sync.Mutex
	connections map[net.Conn]bool
	workers     sync.WaitGroup
	accepted    chan struct{}
	once        sync.Once
}

// NewServer starts serving commands on a private socket owned by the caller.
func NewServer(socket string, ctx Context) (*Server, error) {
	rpcServer := rpc.NewServer()
	if err := rpcServer.RegisterName("Hookd", NewHandler(ctx)); err != nil {
		return nil, err
	}
	listener, err := net.Listen("unix", socket)
	if err != nil {
		return nil, err
	}
	server := &Server{listener: listener, rpc: rpcServer, connections: make(map[net.Conn]bool), accepted: make(chan struct{})}
	go server.serve()
	return server, nil
}

// serve accepts hook-tool connections until the listener closes.
func (s *Server) serve() {
	defer close(s.accepted)
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		s.mu.Lock()
		s.connections[conn] = true
		s.mu.Unlock()
		s.workers.Add(1)
		go s.serveConnection(conn)
	}
}

// serveConnection serves one RPC client and removes it from the active connection set.
func (s *Server) serveConnection(conn net.Conn) {
	defer s.workers.Done()
	s.rpc.ServeConn(conn)
	conn.Close()
	s.mu.Lock()
	delete(s.connections, conn)
	s.mu.Unlock()
}

// Close stops acceptance and closes clients, including abandoned connections.
// It waits for all command handlers and is safe to call more than once.
func (s *Server) Close() {
	s.once.Do(func() {
		// Stop the accept loop before waiting for workers: it is the only
		// goroutine allowed to add workers or register new connections.
		s.listener.Close()
		<-s.accepted

		// Close every accepted connection so an idle or abandoned hook-tool
		// client cannot prevent shutdown. Handlers finish before Close returns.
		s.mu.Lock()
		for conn := range s.connections {
			conn.Close()
		}
		s.mu.Unlock()

		s.workers.Wait()
	})
}
