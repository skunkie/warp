package ssh

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
	"golang.org/x/crypto/ssh"

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
	config  *ssh.ClientConfig
	cli     *ssh.Client
	domains []string
	dns     []string
	ips     []string
	mx      sync.Mutex
}

func New(cfg *Config) (*Protocol, error) {
	sshConfig := &ssh.ClientConfig{
		User: cfg.User,
		Auth: []ssh.AuthMethod{
			ssh.Password(cfg.Password),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         time.Second * 5,
	}

	slog.Debug("open connection", "url", fmt.Sprintf("%s@%s", sshConfig.User, cfg.Host))

	cli, err := ssh.Dial("tcp", cfg.Host+":22", sshConfig)
	if err != nil {
		return nil, err
	}

	var dnsList []string

	if len(cfg.DNS) != 0 {
		dnsList = cfg.DNS
	} else {
		slog.Debug("get dns servers", "url", fmt.Sprintf("%s@%s", sshConfig.User, cfg.Host))

		session, err := cli.NewSession()
		if err != nil {
			return nil, err
		}

		defer session.Close()

		sshDNS, err := session.CombinedOutput("scutil --dns | grep \"nameserver\\[.\\]\" | awk '{print $3}' | head -n 1")
		if err != nil {
			return nil, err
		}

		if len(sshDNS) != 0 {
			dnsList = append(dnsList, strings.TrimSpace(string(sshDNS)))
		}
	}

	return &Protocol{
		host:    cfg.Host,
		config:  sshConfig,
		dns:     dnsList,
		cli:     cli,
		domains: cfg.Domains,
		ips:     cfg.IPs,
	}, nil
}

func (p *Protocol) dial(n, addr string) (net.Conn, error) {
	for i := 0; ; i++ {
		slog.Debug("open dial", "attempt", strconv.Itoa(i), "dest", addr, "type", n)

		conn, err := p.cli.Dial(n, addr)
		if err == nil || i == 2 {
			return conn, err
		}

		if _, ok := err.(net.Error); ok {
			return nil, err
		}

		slog.Warn("reopen connection", "dest", addr, "type", n, "url", fmt.Sprintf("%s@%s", p.config.User, p.host), "error", err)

		if !p.mx.TryLock() {
			time.Sleep(1 * time.Second)

			continue
		}

		cli, err := ssh.Dial("tcp", p.host+":22", p.config)
		if err != nil {
			slog.Error("failed to open ssh session", "error", err)

			p.mx.Unlock()

			return nil, err
		}

		_ = p.cli.Close()
		p.cli = cli

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
