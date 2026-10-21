package player

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"
)

const ipcRequestTimeout = 2 * time.Second

var errIPCClosed = errors.New("mpv IPC connection closed")

// ipcMessage is any line mpv sends: a reply (request_id set) or an event
type ipcMessage struct {
	Error     string `json:"error"`
	Data      any    `json:"data"`
	RequestID int    `json:"request_id"`
	Event     string `json:"event"`
	Name      string `json:"name"` // property-change only
}

// ipcClient is a minimal JSON IPC client for mpv over a single persistent
// socket. Requests are matched to replies by a sequential request_id; events
// (including observed property changes) are handed to onEvent from the read
// loop, so onEvent must not block.
type ipcClient struct {
	conn    net.Conn
	writeMu sync.Mutex

	mu      sync.Mutex
	nextID  int
	pending map[int]chan ipcMessage
	closed  bool

	onEvent func(ipcMessage)
}

func dialIPC(socketPath string, onEvent func(ipcMessage)) (*ipcClient, error) {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return nil, err
	}

	c := &ipcClient{
		conn:    conn,
		pending: make(map[int]chan ipcMessage),
		onEvent: onEvent,
	}
	go c.readLoop()

	return c, nil
}

func (c *ipcClient) readLoop() {
	scanner := bufio.NewScanner(c.conn)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024) // metadata replies can be large

	for scanner.Scan() {
		var msg ipcMessage
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			continue
		}

		if msg.Event != "" {
			if c.onEvent != nil {
				c.onEvent(msg)
			}
			continue
		}

		c.mu.Lock()
		ch, ok := c.pending[msg.RequestID]
		delete(c.pending, msg.RequestID)
		c.mu.Unlock()

		if ok {
			ch <- msg
		}
	}

	// Connection gone: fail everything still waiting
	c.mu.Lock()
	c.closed = true
	for id, ch := range c.pending {
		ch <- ipcMessage{Error: errIPCClosed.Error()}
		delete(c.pending, id)
	}
	c.mu.Unlock()
}

// exec sends a command and waits for its reply
func (c *ipcClient) exec(args ...any) (any, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, errIPCClosed
	}
	c.nextID++
	id := c.nextID
	ch := make(chan ipcMessage, 1)
	c.pending[id] = ch
	c.mu.Unlock()

	line, err := json.Marshal(struct {
		Command   []any `json:"command"`
		RequestID int   `json:"request_id"`
	}{args, id})
	if err != nil {
		c.forget(id)
		return nil, err
	}

	c.writeMu.Lock()
	_, err = c.conn.Write(append(line, '\n'))
	c.writeMu.Unlock()
	if err != nil {
		c.forget(id)
		return nil, err
	}

	select {
	case msg := <-ch:
		if msg.Error != "success" {
			return msg.Data, fmt.Errorf("mpv: %s", msg.Error)
		}
		return msg.Data, nil
	case <-time.After(ipcRequestTimeout):
		c.forget(id)
		return nil, fmt.Errorf("mpv: request %v timed out", args)
	}
}

func (c *ipcClient) forget(id int) {
	c.mu.Lock()
	delete(c.pending, id)
	c.mu.Unlock()
}

func (c *ipcClient) close() {
	_ = c.conn.Close()
}

func (c *ipcClient) setProperty(name string, value any) error {
	_, err := c.exec("set_property", name, value)
	return err
}

func (c *ipcClient) floatProperty(name string) float64 {
	data, err := c.exec("get_property", name)
	if err != nil {
		return 0
	}
	f, _ := data.(float64)
	return f
}
