package mobster

import (
	"testing"
	"log"
)

func TestMobsterStarting(t *testing.T) {
	log.Println("ttt")
	go StartServer(4009)
	log.Println("testing!")
}
