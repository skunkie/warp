package socks5

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/miekg/dns"
	"golang.org/x/net/proxy"

	"github.com/merzzzl/warp/internal/utils/log"
	"github.com/merzzzl/warp/internal/utils/network"
)

type Config struct {
	User     string   `yaml:"user"`
	Password string   `yaml:"password"`
	Host     string   `yaml:"host"`
	Domains  []string `yaml:"domains"`
	IPs      []string `yaml:"ips"`
	DNS      []string `yaml:"dns"`
}

type Protocol struct {
	host    string
	dialer  proxy.Dialer
	auth    *proxy.Auth
	domains []string
	dns     []string
	ips     []string
	mx      sync.Mutex
}

func New(cfg *Config) (*Protocol, error) {
	var auth *proxy.Auth

	if cfg.User != "" && cfg.Password != "" {
		auth = &proxy.Auth{
			User:     cfg.User,
			Password: cfg.Password,
		}
	}

	dialer, err := proxy.SOCKS5("tcp", cfg.Host, auth, proxy.Direct)
	if err != nil {
		return nil, err
	}

	slog.Debug("open connection", "url", fmt.Sprintf("%s", cfg.Host))

	return &Protocol{
		host:    cfg.Host,
		auth:    auth,
		dns:     cfg.DNS,
		dialer:  dialer,
		domains: cfg.Domains,
		ips:     cfg.IPs,
	}, nil
}

func (p *Protocol) dial(n, addr string) (net.Conn, error) {
	for i := 0; ; i++ {
		slog.Debug("open dial", "attempt", strconv.Itoa(i), "dest", addr, "type", n)

		conn, err := p.dialer.Dial(n, addr)
		if err == nil || i == 2 {
			return conn, err
		}

		if _, ok := err.(net.Error); ok {
			return nil, err
		}

		slog.Warn("reopen connection", "dest", addr, "type", n, "url", fmt.Sprintf("%s", p.host), "error", err)

		if !p.mx.TryLock() {
			time.Sleep(1 * time.Second)

			continue
		}

		dialer, err := proxy.SOCKS5("tcp", p.host, p.auth, proxy.Direct)
		if err != nil {
			slog.Error("failed to open socks5 tunnel", "error", err)

			p.mx.Unlock()

			return nil, err
		}

		p.dialer = dialer

		p.mx.Unlock()

		time.Sleep(1 * time.Second)
	}
}

func (p *Protocol) Domains() []string {
	return p.domains
}

func (p *Protocol) FixedIPs() []string {
	return p.ips
}

func (p *Protocol) LookupHost(_ context.Context, req *dns.Msg) *dns.Msg {
	for _, addr := range p.dns {
		dnsConn, err := p.dial("tcp", addr+":53")
		if err != nil {
			slog.Error("handle dns req", append([]any{"server", addr, "error", err}, log.DNSAttrs(req)...)...)

			continue
		}

		co := new(dns.Conn)
		co.Conn = dnsConn

		err = co.WriteMsg(req)
		if err != nil {
			slog.Error("write dns req", append([]any{"server", addr, "error", err}, log.DNSAttrs(req)...)...)

			continue
		}

		rsp, err := co.ReadMsg()
		if err != nil {
			slog.Error("read dns req", append([]any{"server", addr, "error", err}, log.DNSAttrs(req)...)...)

			continue
		}

		slog.Debug("handle dns req", append([]any{"server", addr}, log.DNSAttrs(req)...)...)

		if len(rsp.Answer) == 0 {
			continue
		}

		return rsp
	}

	return req
}

func (p *Protocol) HandleTCP(conn net.Conn) {
	remoteConn, err := p.dial(conn.LocalAddr().Network(), conn.LocalAddr().String())
	if err != nil {
		if !errors.Is(err, io.EOF) {
			slog.Warn("handle conn", "dest", conn.LocalAddr().String(), "type", conn.LocalAddr().Network(), "error", err)
		}

		return
	}

	slog.Info("handle conn", "dest", conn.LocalAddr().String(), "type", conn.LocalAddr().Network())

	network.Transfer(conn, remoteConn)
}
