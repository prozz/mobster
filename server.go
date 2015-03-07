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

type Request struct {
	client *Client
	msg    string
}

type Response struct {
	name string
	msg  string
}

type Server struct {
	port      int
	startTime time.Time

	incomingClients  chan (*Client)
	incomingRequests chan (Request)
	writeToClient    chan (Response)
	writeToRoom      chan (Response)
	disconnects      chan (*Client)

	listener net.Listener

	clientHolder *ClientHolder

	// true when shutdown procedure started, false otherwise
	shutdownMode bool
	// notifies processingLoop about shutdown procedure
	shutdownNow chan (bool)

	shutdownWaitGroup *sync.WaitGroup

	OnConnect    func(ctx Ctx, name, room string)
	OnDisconnect func(ctx Ctx, name, room string)
	OnMessage    func(ctx Ctx, name, room, message string)
}

func NewServer() *Server {
	s := &Server{}

	s.incomingClients = make(chan *Client)
	s.incomingRequests = make(chan Request)
	s.writeToClient = make(chan Response)
	s.writeToRoom = make(chan Response)
	s.disconnects = make(chan *Client)
	s.shutdownNow = make(chan bool)

	s.clientHolder = NewClientHolder()

	s.shutdownWaitGroup = &sync.WaitGroup{}

	s.OnConnect = func(ctx Ctx, name, room string) {
		log.Println("warn: OnConnect default handler")
	}
	s.OnDisconnect = func(ctx Ctx, name, room string) {
		log.Println("warn: OnDisconnect default handler")
	}
	s.OnMessage = func(ctx Ctx, name, room, message string) {
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
			for _, c := range s.clientHolder.GetAll() {
				s.disconnect(c)
			}
			return
		case c := <-s.disconnects:
			log.Printf("[audit] %s: %s disconnects", c.room, c.name)
			s.disconnect(c)
		case c := <-s.incomingClients:
			s.clientHolder.Add(c)
			log.Printf("[audit] %s: %s joins", c.room, c.name)
			s.OnConnect(s, c.name, c.room)
		case r := <-s.incomingRequests:
			log.Printf("[audit] %s: %s -> %s", r.client.room, r.client.name, r.msg)
			s.OnMessage(s, r.client.name, r.client.room, r.msg)
		case r := <-s.writeToClient:
			c := s.clientHolder.GetByName(r.name)
			s.send(c, r.msg)
			log.Printf("[audit] %s: %s <- %s", c.room, c.name, r.msg)
		case r := <-s.writeToRoom:
			for _, c := range s.clientHolder.GetByRoom(r.name) {
				s.send(c, r.msg)
				log.Printf("[audit] %s: %s <- %s", c.room, c.name, r.msg)
			}
		}
	}
}

func (s *Server) send(client *Client, msg string) {
	client.conn.Write([]byte(msg))
}

func (s *Server) disconnect(client *Client) {
	client.conn.Close()
	s.clientHolder.Remove(client)
	s.OnDisconnect(s, client.name, client.room)
}

func (s *Server) DumpStats() {
	log.Printf("uptime: %s, connected clients: %d", time.Since(s.startTime), s.clientHolder.Count())
}

type Ctx interface {
	SendTo(name, message string)
	SendToRoom(name, message string)
}

func (s *Server) SendTo(name, message string) {
	go func() {
		s.writeToClient <- Response{name, message}
	}()
}

func (s *Server) SendToRoom(name, message string) {
	go func() {
		s.writeToRoom <- Response{name, message}
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
