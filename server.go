package mobster

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

type Client struct {
	name string
	room string
	conn net.Conn
}

type Request struct {
	client *Client
	msg    string
}

type Response struct {
	client *Client
	msg string
}

type Server struct {
	port      int
	startTime time.Time

	incomingClients  chan (*Client)
	incomingRequests chan (Request)
	responses        chan (Response)
	disconnects      chan (*Client)

	listener net.Listener

	clients map[*Client]bool
	clientsByName map[string]*Client

	// true when shutdown procedure started, false otherwise
	shutdownMode bool
	// notifies processingLoop about shutdown procedure
	shutdownNow chan (bool)

	shutdownWaitGroup *sync.WaitGroup

	OnConnect    func(name, room string)
	OnDisconnect func(name, room string)
	OnMessage    func(name, room, message string)
}

func NewServer() *Server {
	s := &Server{}

	s.incomingClients = make(chan *Client)
	s.incomingRequests = make(chan Request)
	s.responses = make(chan Response)
	s.disconnects = make(chan *Client)
	s.shutdownNow = make(chan bool)
	s.clients = make(map[*Client]bool)
	s.clientsByName = make(map[string]*Client)

	s.shutdownWaitGroup = &sync.WaitGroup{}

	s.OnConnect = func(name, room string) {
		log.Println("warn: OnConnect default handler")
	}
	s.OnDisconnect = func(name, room string) {
		log.Println("warn: OnDisconnect default handler")
	}
	s.OnMessage = func(name, room, message string) {
		log.Println("warn: OnMessage default handler")
	}

	return s
}

func (s *Server) StartServer(port int) {
	s.port = port
	s.startTime = time.Now()

	log.Printf("starting server on port %d", s.port)
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", s.port))
	if err != nil {
		log.Fatal("cannot listen:", err)
	}

	s.listener = listener

	s.shutdownWaitGroup.Add(2)
	go s.processingLoop()
	go s.acceptingLoop()
}

func (s *Server) StartServerAndWait(port int) {
	s.StartServer(port)

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	for {
		signal := <-signals
		switch signal {
		case syscall.SIGQUIT:
			s.DumpStats()
		default:
			s.StopServer()
			os.Exit(0)
		}
	}
}

func (s *Server) StopServer() {
	log.Printf("shutting down...")
	s.shutdownMode = true
	s.listener.Close()
	s.shutdownNow <- true
	s.shutdownWaitGroup.Wait()
	log.Printf("bye!")
}

func (s *Server) acceptingLoop() {
	defer s.shutdownWaitGroup.Done()
	for {
		conn, err := s.listener.Accept()
		if s.shutdownMode {
			return
		}
		if err != nil {
			log.Println("accept error:", err)
			continue
		}
		s.shutdownWaitGroup.Add(1)
		go s.handleConnection(conn)
	}
}

func (s *Server) handleConnection(conn net.Conn) {
	defer s.shutdownWaitGroup.Done()

	conn.SetDeadline(time.Now().Add(2 * time.Second))
	name, room, err := auth(conn)
	if err != nil {
		log.Println("auth error:", err)
		conn.Close()
		return
	}
	conn.SetDeadline(time.Time{})

	client := &Client{name, room, conn}
	s.clients[client] = true
	s.clientsByName[name] = client
	s.incomingClients <- client

	for {
		var req string
		err := read(&req, client.conn)
		if err != nil {
			log.Println("read error:", err)
			if !s.shutdownMode {
				s.disconnects <- client
			}
			return
		}
		msgs := strings.Split(req, "\n")
		for _, msg := range msgs {
			s.incomingRequests <- Request{client, msg}
		}
	}
}

// extracted to go routine, so that rooms ops are thread safe (adding/removing clients)
func (s *Server) processingLoop() {
	defer s.shutdownWaitGroup.Done()
	for {
		select {
		case <-s.shutdownNow:
			log.Printf("disconnecting all clients")
			for c := range s.clients {
				s.disconnect(c)
			}
			return
		case c := <-s.disconnects:
			log.Printf("[audit] %s: %s disconnects", c.room, c.name)
			s.disconnect(c)
		case c := <-s.incomingClients:
			log.Printf("[audit] %s: %s joins", c.room, c.name)
			s.OnConnect(c.name, c.room)
		case r := <-s.incomingRequests:
			log.Printf("[audit] %s: %s -> %s", r.client.room, r.client.name, r.msg)
			s.OnMessage(r.client.name, r.client.room, r.msg)
		case r := <-s.responses:
			r.client.conn.Write([]byte(r.msg))
			log.Printf("[audit] %s: %s <- %s", r.client.room, r.client.name, r.msg)
		}
	}
}

func (s *Server) disconnect(client *Client) {
	client.conn.Close()
	delete(s.clients, client)
	delete(s.clientsByName, client.name)
	s.OnDisconnect(client.name, client.room)
}

func (s *Server) DumpStats() {
	log.Printf("uptime: %s, connected clients: %d", time.Since(s.startTime), len(s.clients))
}

func (s *Server) SendTo(name, message string) {
	go func() {
		s.responses <- Response{name, message}
	}()
}

// reads initial auth packet from connection
// returns username, room
func auth(conn net.Conn) (string, string, error) {
	var req string
	err := read(&req, conn)
	if err != nil {
		return "", "", err
	}

	tokens := strings.Split(req, " ")
	if len(tokens) != 3 || tokens[0] != "a" {
		return "", "", fmt.Errorf("malformed auth request <%s>", req)
	}

	return tokens[1], tokens[2], nil
}

// reads from connection
func read(msg *string, conn net.Conn) error {
	var buf [512]byte
	n, err := conn.Read(buf[0:])
	if err != nil {
		return err
	}
	*msg = strings.TrimSpace(string(buf[:n]))
	return nil
}
