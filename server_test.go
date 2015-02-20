package mobster

import (
	"log"
	"net"
	"runtime"
	"testing"
	"time"
)

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

func TestStopServer_disconnects(t *testing.T) {
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

func TestFlow_connectionHandler(t *testing.T) {
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

func connectAndSend(t *testing.T, msgs ...string) {
	send(t, connect(t), msgs...)
}

func sleep() {
	time.Sleep(50 * time.Millisecond)
}
