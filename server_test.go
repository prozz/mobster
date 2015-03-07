package mobster

import (
	"flag"
	"io/ioutil"
	"log"
	"net"
	"runtime"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	flag.Parse()
	if !testing.Verbose() {
		log.SetOutput(ioutil.Discard)
	}
	m.Run()
}

func TestStopServer_goroutines(t *testing.T) {
	before := runtime.NumGoroutine()
	s := NewServer()
	s.StartServer(4009)
	during := runtime.NumGoroutine()
	if before >= during {
		t.Error("server doesnt fire properly, as no new goroutines")
	}
	s.StopServer()
	after := runtime.NumGoroutine()
	if after != before {
		t.Error("server goroutines should stop")
	}

	log.Printf("goroutines count: before %d, during %d, after %d", before, during, after)
}

func TestStopServer_disconnectHandler(t *testing.T) {
	connections := 0
	s := NewServer()
	s.OnConnect = func(name, room string) {
		connections++
	}
	s.OnDisconnect = func(name, room string) {
		connections--
	}
	s.StartServer(4009)

	connectAndSend(t, "a foo 123")
	connectAndSend(t, "a bar 123")

	if connections != 2 {
		t.Error("exactly two clients should be connected now")
	}

	s.StopServer()

	if connections != 0 {
		t.Error("server should disconnect ongoing clients")
	}
}

func TestFlow_connectHandler(t *testing.T) {
	connected := false
	s := NewServer()
	s.OnConnect = func(name, room string) {
		connected = true
	}
	s.StartServer(4009)

	connectAndSend(t, "a foo 123")

	if !connected {
		t.Error("on connect did not fire")
	}
	s.StopServer()
}

func TestFlow_remoteHardDisconnect(t *testing.T) {
	called := false
	s := NewServer()
	s.OnDisconnect = func(name, room string) {
		called = true
	}
	s.StartServer(4009)

	c := connectAndSend(t, "a foo 123")
	c.Close()
	sleep()

	if !called {
		t.Error("no OnDisconnect called after remote disconnect")
	}

	s.StopServer()
}

func TestFlow_messageHandler(t *testing.T) {
	called := false
	s := NewServer()
	s.OnMessage = func(name, room, message string) {
		called = true
	}
	s.StartServer(4009)

	c := connectAndSend(t, "a foo 123")
	send(t, c, "foo bar")

	if !called {
		t.Error("no OnMessage called after sending message")
	}

	s.StopServer()
}

func TestFlow_messageHandlerResponse(t *testing.T) {
	s := NewServer()
	s.OnMessage = func(name, room, message string) {
		s.SendTo(name, message)
	}
	s.StartServer(4009)

	c := connectAndSend(t, "a foo 123")
	send(t, c, "foo bar")
	response := readFromServer(t, c)

	if response != "foo bar" {
		t.Error("no valid response, should echo what was send")
	}

	s.StopServer()
}

func connect(t *testing.T) net.Conn {
	conn, err := net.Dial("tcp", "127.0.0.1:4009")
	if err != nil {
		t.Fatal(err)
	}
	return conn
}

func send(t *testing.T, conn net.Conn, msgs ...string) {
	for _, msg := range msgs {
		_, e := conn.Write([]byte(msg))
		if e != nil {
			t.Errorf("cannot send msg: %s", msg)
		}
		// we are faster locally so have to delay stuff a little
		sleep()
	}
}

func connectAndSend(t *testing.T, msgs ...string) net.Conn {
	conn := connect(t)
	send(t, conn, msgs...)
	return conn
}

func readFromServer(t *testing.T, conn net.Conn) string {
	conn.SetDeadline(time.Now().Add(50 * time.Millisecond))
	var buf [512]byte
	n, err := conn.Read(buf[0:])
	if err != nil {
		t.Errorf("cannot read: %s", err)
		return ""
	}
	return string(buf[:n])
}

func sleep() {
	time.Sleep(50 * time.Millisecond)
}
