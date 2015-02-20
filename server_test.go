package mobster

import (
	"testing"
	"log"
	"time"
	"net"
)

func sleep() {
	time.Sleep(50 * time.Millisecond)
}

func TestMobsterBasicFlow(t *testing.T) {
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

func TestMobsterXXX(t *testing.T) {
	log.Println("222")
	s := NewServer()
	s.StartServer(4009)
	time.Sleep(1 * time.Second)

	_, err := net.Dial("tcp", "127.0.0.1:4009")
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(1 * time.Second)

	log.Println("testing!")
	s.StopServer()
}
