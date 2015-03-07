package mobster

import "testing"

func TestClientHolder_AddAndRemove(t *testing.T) {
	h := NewClientHolder()
	c := &Client{}

	h.Add(c)
	if h.Count() != 1 {
		t.Error("expected one client")
	}

	h.Remove(c)
	if h.Count() != 0 {
		t.Error("expected no clients")
	}
}

func TestClientHolder_GetAll(t *testing.T) {
	h := NewClientHolder()
	c1 := &Client{name: "foo"}
	c2 := &Client{name: "bar"}

	h.Add(c1)
	h.Add(c2)

	r := h.GetAll()
	if len(r) != 2 {
		t.Error("expected two clients")
	}

	h.Remove(c1)
	r = h.GetAll()
	if len(r) != 1 {
		t.Error("expected one client")
	}
}

func TestClientHolder_GetByName(t *testing.T) {
	h := NewClientHolder()
	c1 := &Client{name: "foo"}
	c2 := &Client{name: "bar"}

	h.Add(c1)
	h.Add(c2)

	r := h.GetByName("foo")
	if r != c1 {
		t.Error("wrong client")
	}

	h.Remove(c1)
	r = h.GetByName("foo")
	if r != nil {
		t.Error("not cleaned up")
	}
}

func TestClientHolder_GetByRoom(t *testing.T) {
	h := NewClientHolder()
	c1 := &Client{name: "foo", room: "1"}
	c2 := &Client{name: "bar", room: "1"}

	h.Add(c1)
	h.Add(c2)

	r := h.GetByRoom("1")
	if len(r) != 2 {
		t.Error("no proper clients in room")
	}

	h.Remove(c1)
	r = h.GetByRoom("1")
	if len(r) != 1 {
		t.Error("remove from room failure")
	}
	if r[0] != c2 {
		t.Error("removed wrong client")
	}
}
