package mobster

import "net"

type Client struct {
	user string
	room string
	conn net.Conn
}

type ClientHolder struct {
	clients       map[*Client]bool
	clientsByName map[string]*Client
	clientsByRoom map[string][]*Client
}

func NewClientHolder() *ClientHolder {
	h := new(ClientHolder)
	h.clients = make(map[*Client]bool)
	h.clientsByName = make(map[string]*Client)
	h.clientsByRoom = make(map[string][]*Client)
	return h
}

func (h *ClientHolder) Add(c *Client) {
	h.clients[c] = true
	h.clientsByName[c.user] = c
	h.clientsByRoom[c.room] = append(h.clientsByRoom[c.room], c)
}

func (h *ClientHolder) Remove(c *Client) {
	room := h.clientsByRoom[c.room]
	var pos int
	for idx, client := range room {
		if client == c {
			pos = idx
			break
		}
	}
	h.clientsByRoom[c.room] = append(room[:pos], room[pos+1:]...)

	delete(h.clientsByName, c.user)
	delete(h.clients, c)
}

func (h *ClientHolder) RemoveRoom(room string) {
	clients := h.clientsByRoom[room]
	for _, c := range clients {
		delete(h.clientsByName, c.user)
		delete(h.clients, c)
	}
	delete(h.clientsByRoom, room)
}

func (h *ClientHolder) GetAll() []*Client {
	var clients = []*Client{}
	for c := range h.clients {
		clients = append(clients, c)
	}
	return clients
}

func (h *ClientHolder) GetByName(user string) *Client {
	return h.clientsByName[user]
}

func (h *ClientHolder) GetByRoom(room string) []*Client {
	return h.clientsByRoom[room]
}

func (h *ClientHolder) GetRoomUsers(room string) []string {
	var users []string
	for _, c := range h.clientsByRoom[room] {
		users = append(users, c.user)
	}
	return users
}

func (h *ClientHolder) GetRoomCount(room string) int {
	return len(h.clientsByRoom[room])
}

func (h *ClientHolder) Count() int {
	return len(h.clients)
}
