package mobster

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
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
}

type Server struct {
	port      int
	startTime time.Time

	incomingClients      chan (*Client)
	disconnectingClients chan (*Client)

	incomingRequests chan (Request)
	responses        chan (Response)

	listener net.Listener

	clients map[*Client]bool

	// notifies goroutines about shutdown procedure
	shutdownNow chan(chan bool)
	// true when shutdown procedure started, false otherwise
	shutdownMode bool

	OnConnect    func(name, room string)
	OnDisconnect func(name, room string)
	OnMessage    func(name, room, message string)
}

func NewServer() *Server {
	s := &Server{}

	s.incomingClients = make(chan *Client)
	s.disconnectingClients = make(chan *Client)
	s.incomingRequests = make(chan Request)
	s.shutdownNow = make(chan (chan bool))

	s.clients = make(map[*Client]bool)

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

	done := make(chan bool, 1)
	s.shutdownNow <- done
	<-done
}

func (s *Server) acceptingLoop() {
	for {
		conn, err := s.listener.Accept()
		if s.shutdownMode {
			return
		}
		if err != nil {
			log.Println("accept error:", err)
			continue
		}
		go handleConnection(s, conn)
	}

}

func handleConnection(s *Server, conn net.Conn) {
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
	s.incomingClients <- client

	go s.clientReaderLoop(client)
}

func (s *Server) clientReaderLoop(client *Client) {
	for {
		var req string
		err := read(&req, client.conn)
		if err != nil {
			log.Println("read error:", err)
			s.disconnectingClients <- client
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
	for {
		select {
		case done :=  <-s.shutdownNow:
			for c := range s.clients {
				c.conn.Close()
				delete(s.clients, c)
				log.Printf("[audit] %s: %s force disconnects", c.room, c.name)
				s.OnDisconnect(c.name, c.room)
			}
			done <- true
			return
		case c := <-s.incomingClients:
			log.Printf("[audit] %s: %s joins", c.room, c.name)
			s.OnConnect(c.name, c.room)
		case c := <-s.disconnectingClients:
			c.conn.Close()
			delete(s.clients, c)
			log.Printf("[audit] %s: %s disconnects", c.room, c.name)
			s.OnDisconnect(c.name, c.room)
		case r := <-s.incomingRequests:
			log.Printf("[audit] %s: %s -> %s", r.client.room, r.client.name, r.msg)
			s.OnMessage(r.client.name, r.client.room, r.msg)

		case r := <-s.responses:
			log.Printf("[audit] xxx %s", r)
		}
	}
}

func (s *Server) DumpStats() {
	log.Printf("uptime: %s", time.Since(s.startTime))
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
