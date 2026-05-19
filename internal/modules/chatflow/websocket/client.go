package websocket

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

// IPRateLimiter maintains a map of rate limiters per IP.
type IPRateLimiter struct {
	ips             map[string]*visitor
	mu              sync.Mutex
	r               rate.Limit
	b               int
	cleanupInterval time.Duration
	visitorTTL      time.Duration
	quit            chan struct{}
}

type visitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// NewIPRateLimiter creates a new rate limiter with background cleanup.
func NewIPRateLimiter(r rate.Limit, b int) *IPRateLimiter {
	i := &IPRateLimiter{
		ips:             make(map[string]*visitor),
		r:               r,
		b:               b,
		cleanupInterval: time.Minute,
		visitorTTL:      3 * time.Minute,
		quit:            make(chan struct{}),
	}
	go i.cleanupVisitors()
	return i
}

// Stop stops the background cleanup goroutine.
func (i *IPRateLimiter) Stop() {
	close(i.quit)
}

// GetLimiter returns the rate limiter for a given IP.
func (i *IPRateLimiter) GetLimiter(ip string) *rate.Limiter {
	i.mu.Lock()
	defer i.mu.Unlock()

	v, exists := i.ips[ip]
	if !exists {
		limiter := rate.NewLimiter(i.r, i.b)
		i.ips[ip] = &visitor{limiter: limiter, lastSeen: time.Now()}
		return limiter
	}

	v.lastSeen = time.Now()
	return v.limiter
}

func (i *IPRateLimiter) cleanupVisitors() {
	ticker := time.NewTicker(i.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			i.mu.Lock()
			for ip, v := range i.ips {
				if time.Since(v.lastSeen) > i.visitorTTL {
					delete(i.ips, ip)
				}
			}
			i.mu.Unlock()
		case <-i.quit:
			return
		}
	}
}

// Global connection limiter: allow 2 connections per second, with a burst of 5.
var connectionLimiter = NewIPRateLimiter(rate.Limit(2), 5)

var trustedProxies = []string{
	"127.0.0.1/8",
	"::1/128",
	"10.0.0.0/8",
	"172.16.0.0/12",
	"192.168.0.0/16",
}

var trustedProxyNets []*net.IPNet

func init() {
	for _, cidr := range trustedProxies {
		_, ipnet, err := net.ParseCIDR(cidr)
		if err == nil {
			trustedProxyNets = append(trustedProxyNets, ipnet)
		}
	}
}

func isTrustedProxy(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	for _, network := range trustedProxyNets {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// getIP extracts the client IP address from the request.
func getIP(r *http.Request) string {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}

	if isTrustedProxy(ip) {
		if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
			ips := strings.Split(forwarded, ",")
			return strings.TrimSpace(ips[0])
		}
		if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
			return strings.TrimSpace(realIP)
		}
	}
	return ip
}

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 512
)

type Client struct {
	hub    *Hub
	conn   *websocket.Conn
	send   chan []byte
	logger *zap.Logger
}

func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()
	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				c.logger.Error("WebSocket unexpected close error", zap.Error(err))
			}
			break
		}
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			n := len(c.send)
			for i := 0; i < n; i++ {
				w.Write(<-c.send)
			}

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// ServeWs handles WebSocket requests from clients.
func ServeWs(hub *Hub, logger *zap.Logger, allowedOrigins []string, w http.ResponseWriter, r *http.Request) {
	ip := getIP(r)
	limiter := connectionLimiter.GetLimiter(ip)
	if !limiter.Allow() {
		logger.Warn("WebSocket connection rate limit exceeded", zap.String("ip", ip))
		http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
		return
	}

	var upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			origin := r.Header.Get("Origin")
			if origin == "" {
				return true
			}
			for _, allowed := range allowedOrigins {
				if allowed == "*" || origin == allowed {
					return true
				}
			}
			return false
		},
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Error("Failed to upgrade to WebSocket", zap.Error(err))
		return
	}

	client := &Client{
		hub:    hub,
		conn:   conn,
		send:   make(chan []byte, 256),
		logger: logger,
	}

	client.hub.register <- client

	go client.writePump()
	go client.readPump()

	logger.Info("New WebSocket connection established")
}
