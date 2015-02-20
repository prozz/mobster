package mobster

import (
	"testing"
	"log"
	"time"
	"net"
	"runtime"
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
	connected := 0

	s := NewServer()
	s.OnConnect = func(name, room string) {
		connected += 1
	}
	s.OnDisconnect = func(name, room string) {
		connected -= 1
	}
	s.StartServer(4009)

	sleep()

	conn, err := net.Dial("tcp", "127.0.0.1:4009")
	if err != nil {
		t.Fatal(err)
	}

	_, e := conn.Write([]byte("a prozz 123"))
	if e != nil {
		t.Error("cannot send auth msg")
	}

	sleep()

	if connected != 1 {
		t.Error("one client should be connected")
	}
	s.StopServer()

	if connected != 0 {
		t.Fatal("no disconnect")
	}

}

func sleep() {
	time.Sleep(50 * time.Millisecond)
}
