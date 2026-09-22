package quoteprotocol

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

type Config struct {
	Addr              string
	Username          string
	Password          string
	DialTimeout       time.Duration
	RequestTimeout    time.Duration
	HeartbeatInterval time.Duration
	ReconnectInterval time.Duration
	MaxSkipPackets    int
}

type Request struct {
	Type       uint16
	ExpectType uint16
	Size       int16
	Option     int16
	Payload    []byte
	Timeout    time.Duration
}

type PushHandler func(*Response)

type Client struct {
	cfg Config

	mu           sync.Mutex
	conn         net.Conn
	started      bool
	closed       bool
	reconnecting bool
	nextIndex    byte
	pending      map[byte]*pendingRequest
	cancel       context.CancelFunc
	runCtx       context.Context
	pushHandler  PushHandler
	lastReadUnix atomic.Int64
	heartbeats   atomic.Uint64
	reconnects   atomic.Uint64
}

type pendingRequest struct {
	index      byte
	expectType uint16
	ch         chan requestResult
}

type requestResult struct {
	resp *Response
	err  error
}

type Stats struct {
	Started        bool      `json:"started"`
	Connected      bool      `json:"connected"`
	Reconnecting   bool      `json:"reconnecting"`
	Pending        int       `json:"pending"`
	LastReadTime   time.Time `json:"last_read_time,omitempty"`
	HeartbeatCount uint64    `json:"heartbeat_count"`
	ReconnectCount uint64    `json:"reconnect_count"`
}

func New(cfg Config) *Client {
	if cfg.Addr == "" {
		cfg.Addr = "222.66.65.68:8888"
	}
	if cfg.Username == "" {
		cfg.Username = "anyone"
	}
	if cfg.Password == "" {
		cfg.Password = "anyone"
	}
	if cfg.DialTimeout <= 0 {
		cfg.DialTimeout = 5 * time.Second
	}
	if cfg.RequestTimeout <= 0 {
		cfg.RequestTimeout = 5 * time.Second
	}
	if cfg.HeartbeatInterval <= 0 {
		cfg.HeartbeatInterval = 20 * time.Second
	}
	if cfg.ReconnectInterval <= 0 {
		cfg.ReconnectInterval = 3 * time.Second
	}
	if cfg.MaxSkipPackets <= 0 {
		cfg.MaxSkipPackets = 16
	}
	return &Client{
		cfg:     cfg,
		pending: map[byte]*pendingRequest{},
	}
}

func (c *Client) SetPushHandler(handler PushHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pushHandler = handler
}

func (c *Client) Start(ctx context.Context) error {
	c.mu.Lock()
	if c.started && !c.closed {
		c.mu.Unlock()
		return nil
	}
	runCtx, cancel := context.WithCancel(ctx)
	c.cancel = cancel
	c.runCtx = runCtx
	c.closed = false
	c.started = true
	c.mu.Unlock()

	conn, loginResp, err := c.connectAndLogin(runCtx)
	if err != nil {
		c.mu.Lock()
		c.started = false
		c.closed = true
		c.cancel = nil
		c.runCtx = nil
		c.mu.Unlock()
		cancel()
		return err
	}
	_ = loginResp

	c.mu.Lock()
	c.conn = conn
	c.lastReadUnix.Store(time.Now().Unix())
	c.mu.Unlock()

	go c.readLoop(runCtx, conn)
	go c.heartbeatLoop(runCtx)
	return nil
}

func (c *Client) Close() error {
	c.mu.Lock()
	c.closed = true
	if c.cancel != nil {
		c.cancel()
	}
	conn := c.conn
	c.conn = nil
	c.runCtx = nil
	c.failPendingLocked(errors.New("quote protocol client closed"))
	c.mu.Unlock()
	if conn != nil {
		return conn.Close()
	}
	return nil
}

func (c *Client) LastReadTime() time.Time {
	unix := c.lastReadUnix.Load()
	if unix == 0 {
		return time.Time{}
	}
	return time.Unix(unix, 0)
}

func (c *Client) Stats() Stats {
	c.mu.Lock()
	stats := Stats{
		Started:      c.started && !c.closed,
		Connected:    c.conn != nil && !c.closed,
		Reconnecting: c.reconnecting,
		Pending:      len(c.pending),
	}
	c.mu.Unlock()
	stats.LastReadTime = c.LastReadTime()
	stats.HeartbeatCount = c.heartbeats.Load()
	stats.ReconnectCount = c.reconnects.Load()
	return stats
}

func (c *Client) Request(ctx context.Context, req Request) (*Response, error) {
	if req.Type == 0 {
		return nil, errors.New("request type is required")
	}
	if req.ExpectType == 0 {
		req.ExpectType = req.Type
	}
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = c.cfg.RequestTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	index, pending, conn, err := c.prepareRequest(req.ExpectType)
	if err != nil {
		return nil, err
	}
	body := BuildAskData(req.Type, index, req.Size, req.Option, req.Payload)
	if err := c.writePacket(conn, body); err != nil {
		c.removePending(index)
		c.triggerReconnect()
		return nil, err
	}

	select {
	case result := <-pending.ch:
		return result.resp, result.err
	case <-ctx.Done():
		c.removePending(index)
		return nil, ctx.Err()
	}
}

func (c *Client) LoginProbe(ctx context.Context) (*LoginResponse, error) {
	conn, resp, err := c.connectAndLogin(ctx)
	if conn != nil {
		_ = conn.Close()
	}
	return resp, err
}

func (c *Client) prepareRequest(expectType uint16) (byte, *pendingRequest, net.Conn, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || !c.started {
		return 0, nil, nil, errors.New("quote protocol client is not started")
	}
	if c.conn == nil {
		return 0, nil, nil, errors.New("quote protocol client is disconnected")
	}
	for i := 0; i < 255; i++ {
		c.nextIndex++
		if c.nextIndex == 0 {
			c.nextIndex = 1
		}
		if _, exists := c.pending[c.nextIndex]; !exists {
			p := &pendingRequest{
				index:      c.nextIndex,
				expectType: expectType,
				ch:         make(chan requestResult, 1),
			}
			c.pending[c.nextIndex] = p
			return c.nextIndex, p, c.conn, nil
		}
	}
	return 0, nil, nil, errors.New("too many pending requests")
}

func (c *Client) removePending(index byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.pending, index)
}

func (c *Client) connectAndLogin(ctx context.Context) (net.Conn, *LoginResponse, error) {
	dialer := net.Dialer{Timeout: c.cfg.DialTimeout, KeepAlive: 30 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", c.cfg.Addr)
	if err != nil {
		return nil, nil, err
	}
	_ = conn.SetDeadline(time.Now().Add(c.cfg.RequestTimeout))
	if _, err := conn.Write(BuildWirePacket(BuildLogin(c.cfg.Username, c.cfg.Password, 0))); err != nil {
		_ = conn.Close()
		return nil, nil, err
	}
	resp, err := ReadResponse(conn)
	if err != nil {
		_ = conn.Close()
		return nil, nil, err
	}
	_ = conn.SetDeadline(time.Time{})
	loginResp, err := ParseLoginResponse(resp)
	if err != nil {
		_ = conn.Close()
		return nil, nil, err
	}
	return conn, loginResp, nil
}

func (c *Client) readLoop(ctx context.Context, conn net.Conn) {
	for {
		resp, err := ReadResponse(conn)
		if err != nil {
			if ctx.Err() == nil {
				c.handleConnectionError(conn, err)
			}
			return
		}
		c.lastReadUnix.Store(time.Now().Unix())
		c.dispatch(resp)
	}
}

func (c *Client) dispatch(resp *Response) {
	c.mu.Lock()
	if resp.DataHead != nil {
		if pending := c.pending[resp.DataHead.Index]; pending != nil && pending.expectType == resp.Type {
			delete(c.pending, resp.DataHead.Index)
			c.mu.Unlock()
			pending.ch <- requestResult{resp: resp}
			return
		}
	}
	for index, pending := range c.pending {
		if pending.expectType == resp.Type {
			delete(c.pending, index)
			c.mu.Unlock()
			pending.ch <- requestResult{resp: resp}
			return
		}
	}
	handler := c.pushHandler
	c.mu.Unlock()
	if handler != nil {
		handler(resp)
	}
}

func (c *Client) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(c.cfg.HeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := c.SendHeartbeat(); err != nil {
				c.triggerReconnect()
			}
		}
	}
}

func (c *Client) SendHeartbeat() error {
	c.mu.Lock()
	conn := c.conn
	if conn == nil || c.closed {
		c.mu.Unlock()
		return errors.New("quote protocol client is disconnected")
	}
	c.nextIndex++
	index := c.nextIndex
	c.mu.Unlock()
	if err := c.writePacket(conn, BuildKeepAlive(index)); err != nil {
		return err
	}
	c.heartbeats.Add(1)
	return nil
}

func (c *Client) writePacket(conn net.Conn, body []byte) error {
	if conn == nil {
		return errors.New("nil connection")
	}
	_, err := conn.Write(BuildWirePacket(body))
	return err
}

func (c *Client) handleConnectionError(conn net.Conn, err error) {
	c.mu.Lock()
	if c.conn == conn {
		_ = conn.Close()
		c.conn = nil
		c.failPendingLocked(fmt.Errorf("connection lost: %w", err))
	}
	c.mu.Unlock()
	c.triggerReconnect()
}

func (c *Client) triggerReconnect() {
	c.mu.Lock()
	if c.closed || c.reconnecting {
		c.mu.Unlock()
		return
	}
	c.reconnecting = true
	cancel := c.cancel
	c.mu.Unlock()
	go c.reconnectLoop(cancel)
}

func (c *Client) reconnectLoop(cancel context.CancelFunc) {
	defer func() {
		c.mu.Lock()
		c.reconnecting = false
		c.mu.Unlock()
	}()
	for {
		c.mu.Lock()
		closed := c.closed
		c.mu.Unlock()
		if closed {
			return
		}
		ctx, stop := context.WithTimeout(context.Background(), c.cfg.DialTimeout+c.cfg.RequestTimeout)
		conn, _, err := c.connectAndLogin(ctx)
		stop()
		if err == nil {
			c.mu.Lock()
			if c.closed {
				c.mu.Unlock()
				_ = conn.Close()
				return
			}
			c.conn = conn
			runCtx := c.runCtx
			c.lastReadUnix.Store(time.Now().Unix())
			c.reconnects.Add(1)
			c.mu.Unlock()
			if cancel != nil && runCtx != nil {
				go c.readLoop(runCtx, conn)
			}
			return
		}
		time.Sleep(c.cfg.ReconnectInterval)
	}
}

func (c *Client) failPendingLocked(err error) {
	for index, pending := range c.pending {
		delete(c.pending, index)
		pending.ch <- requestResult{err: err}
	}
}
