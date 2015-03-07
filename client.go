package mobster

import "net"

type Client struct {
	name string
	room string
	conn net.Conn
}

type ClientHolder struct {
	clients map[*Client]bool
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
	h.clientsByName[c.name] = c
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

	delete(h.clientsByName, c.name)
	delete(h.clients, c)
}

func (h *ClientHolder) GetByName(name string) *Client {
	return h.clientsByName[name]
}

func (h *ClientHolder) GetByRoom(name string) []*Client {
	return h.clientsByRoom[name]
}

func (h *ClientHolder) Count() int {
	return len(h.clients)
}
