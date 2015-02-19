package mobster

import (
	"fmt"
	"log"
	"net"
	"strings"
	"time"
	"os"
	"os/signal"
	"syscall"
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
	responses chan (Response)
}

func StartServer(port int) {
	s := &Server{}
	s.port = port
	s.startTime = time.Now()

	s.incomingClients = make(chan *Client)
	s.disconnectingClients = make(chan *Client)
	s.incomingRequests = make(chan Request)

	go s.processingLoop()
	go s.acceptingLoop()

	// signals handling, makes above loops go forever
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	for {
		signal := <-signals
		switch signal {
		case syscall.SIGQUIT:
			s.DumpStats()
		default:
			s.Stop()
			os.Exit(0)
		}
	}
}

func (s *Server) acceptingLoop() {
	log.Printf("starting server on port %d", s.port)
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", s.port))
	if err != nil {
		log.Fatal("cannot listen:", err)
	}

	for {
		conn, err := listener.Accept()
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
		case c := <-s.incomingClients:
			log.Printf("[audit] %s: %s joins", c.room, c.name)
		case c := <-s.disconnectingClients:
			c.conn.Close()
			log.Printf("[audit] %s: %s disconnects", c.room, c.name)
		case r := <-s.incomingRequests:
			log.Printf("[audit] %s: %s -> %s", r.client.room, r.client.name, r.msg)

		case r := <-s.responses:
			log.Printf("[audit] xxx %s", r)
		}
	}
}
func (s *Server) DumpStats() {
	log.Printf("uptime: %s", time.Since(s.startTime))
}

func (s *Server) Stop() {
	log.Printf("shutting down...")
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
