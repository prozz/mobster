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
	client  *Client
	message string
}

type Response struct {
	name, message string // name of either user or room
}

type Server struct {
	startTime time.Time

	incomingClients  chan (*Client)
	incomingRequests chan (Request)

	responses       chan (Response)
	responsesToRoom chan (Response)
	disconnects     chan (string) // name of user to disconnect

	listener net.Listener

	clientHolder *ClientHolder

	// true when shutdown procedure started, false otherwise
	shutdownMode bool
	// notifies processingLoop about shutdown procedure
	shutdownNow chan (bool)

	shutdownWaitGroup *sync.WaitGroup

	// if true there will be no timeout for auth packet
	Debug bool

	OnAuth       func(message string) (username, room string, err error)
	OnConnect    func(ops *Ops, user, room string)
	OnDisconnect func(ops *Ops, user, room string)
	OnMessage    func(ops *Ops, user, room, message string)
}

func NewServer() *Server {
	s := &Server{}

	s.clientHolder = NewClientHolder()

	s.incomingClients = make(chan *Client)
	s.incomingRequests = make(chan Request)

	s.responses = make(chan Response)
	s.responsesToRoom = make(chan Response)
	s.disconnects = make(chan string)

	s.shutdownNow = make(chan bool)
	s.shutdownWaitGroup = &sync.WaitGroup{}

	// default auth function accepts packets like "a <username> <room>"
	s.OnAuth = func(message string) (username, room string, err error) {
		tokens := strings.Split(message, " ")
		if len(tokens) != 3 || tokens[0] != "a" {
			return "", "", fmt.Errorf("malformed auth request <%s>", message)
		}
		return tokens[1], tokens[2], nil
	}
	s.OnConnect = func(ops *Ops, user, room string) {
		log.Println("warn: OnConnect default handler")
	}
	s.OnDisconnect = func(ops *Ops, user, room string) {
		log.Println("warn: OnDisconnect default handler")
	}
	s.OnMessage = func(ops *Ops, user, room, message string) {
		log.Println("warn: OnMessage default handler")
	}

	return s
}

func (s *Server) StartServer(port int) {
	s.startTime = time.Now()

	log.Printf("starting server on port %d", port)
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
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

	log.Println("new connection:", conn.RemoteAddr().String())
	if !s.Debug {
		conn.SetDeadline(time.Now().Add(1 * time.Second))
	}
	var req string
	err := read(&req, conn)
	if err != nil {
		log.Println("cannot read auth packet:", err)
		conn.Close()
		return
	}
	conn.SetDeadline(time.Time{})

	user, room, err := s.OnAuth(req)
	if err != nil {
		log.Println("auth error:", err)
		conn.Close()
		return
	}

	client := &Client{user, room, conn}
	s.incomingClients <- client

	for {
		var req string
		err := read(&req, client.conn)
		if err != nil {
			if !s.shutdownMode {
				log.Println("read error:", err)
				s.disconnects <- user
			}
			return
		}
		messages := strings.Split(req, "\n")
		for _, message := range messages {
			s.incomingRequests <- Request{client, message}
		}
	}
}

// extracted to go routine, so that rooms ops are thread safe (adding/removing clients)
func (s *Server) processingLoop() {
	defer s.shutdownWaitGroup.Done()
	ops := &Ops{s}
	for {
		select {
		case <-s.shutdownNow:
			log.Printf("disconnecting all clients")
			for _, c := range s.clientHolder.GetAll() {
				c.conn.Close()
				s.clientHolder.Remove(c)
				s.OnDisconnect(ops, c.user, c.room)
			}
			return
		case c := <-s.incomingClients:
			s.clientHolder.Add(c)
			log.Printf("[audit] %s: %s joins", c.room, c.user)
			s.OnConnect(ops, c.user, c.room)
		case r := <-s.incomingRequests:
			log.Printf("[audit] %s: %s -> %s", r.client.room, r.client.user, r.message)
			s.OnMessage(ops, r.client.user, r.client.room, r.message)

		// async requests from calls outside handlers
		case user := <-s.disconnects:
			c := s.clientHolder.GetByName(user)
			// may be nil when ops disconnect is used and then accepting loop read nothing
			if c != nil {
				log.Printf("[audit] %s: %s disconnects", c.room, c.user)
				c.conn.Close()
				s.clientHolder.Remove(c)
				s.OnDisconnect(ops, c.user, c.room)
			}
		case r := <-s.responses:
			c := s.clientHolder.GetByName(r.name)
			// may be nil when already disconnected and async server call is used
			if c != nil {
				c.conn.Write([]byte(r.message))
				log.Printf("[audit] %s: %s <- %s", c.room, c.user, r.message)
			}
		case r := <-s.responsesToRoom:
			for _, c := range s.clientHolder.GetByRoom(r.name) {
				c.conn.Write([]byte(r.message))
				log.Printf("[audit] %s: %s <- %s", c.room, c.user, r.message)
			}
		}
	}
}

func (s *Server) DumpStats() {
	log.Printf("uptime: %s, connected clients: %d", time.Since(s.startTime), s.clientHolder.Count())
}

// server operations that may be called from inside OnConnect, OnDisconnect, OnMessage
type Ops struct {
	server *Server
}

// send message to given user
func (o *Ops) SendTo(user, message string) {
	c := o.server.clientHolder.GetByName(user)
	c.conn.Write([]byte(message))
	log.Printf("[audit] %s: %s <- %s", c.room, user, message)
}

// send message to all users in given room
func (o *Ops) SendToRoom(room, message string) {
	for _, c := range o.server.clientHolder.GetByRoom(room) {
		c.conn.Write([]byte(message))
		log.Printf("[audit] %s: %s <- %s", room, c.user, message)
	}
}

// disconnect user
func (o *Ops) Disconnect(user string) {
	c := o.server.clientHolder.GetByName(user)
	c.conn.Close()
	o.server.clientHolder.Remove(c)
	log.Printf("[audit] %s: %s disconnects", c.room, c.user)
}

// disconnect all users in room
func (o *Ops) DisconnectRoom(room string) {
	for _, user := range o.server.clientHolder.GetRoomUsers(room) {
		o.Disconnect(user)
	}
}

// get names of all users in given room
func (o *Ops) GetRoomUsers(room string) []string {
	return o.server.clientHolder.GetRoomUsers(room)
}

// get number of users in given room
func (o *Ops) GetRoomCount(room string) int {
	return o.server.clientHolder.GetRoomCount(room)
}

func (s *Server) SendTo(user, message string) {
	go func() { s.responses <- Response{user, message} }()
}

func (s *Server) SendToRoom(room, message string) {
	go func() { s.responsesToRoom <- Response{room, message} }()
}

func (s *Server) Disconnect(user string) {
	go func() { s.disconnects <- user }()
}

// reads from connection
func read(message *string, conn net.Conn) error {
	var buf [512]byte
	n, err := conn.Read(buf[0:])
	if err != nil {
		return err
	}
	*message = strings.TrimSpace(string(buf[:n]))
	return nil
}
