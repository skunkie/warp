package wg

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/netip"
	"strconv"

	"github.com/MakeNowJust/heredoc"
	"github.com/miekg/dns"
	wgconn "golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun/netstack"

	"github.com/merzzzl/warp/internal/utils/log"
	"github.com/merzzzl/warp/internal/utils/network"
)

var errKeyInvalid = errors.New("invalid wireguard key")

type Config struct {
	PrivateKey    string   `yaml:"private_key"`
	PeerPublicKey string   `yaml:"peer_public_key"`
	Endpoint      string   `yaml:"endpoint"`
	Domains       []string `yaml:"domains"`
	Address       string   `yaml:"address"`
	DNS           []string `yaml:"dns"`
	IPs           []string `yaml:"ips"`
}

type Protocol struct {
	tnet    *netstack.Net
	domains []string
	dns     []string
	ips     []string
}

var defaultMTU = 1480

func New(ctx context.Context, cfg *Config) (*Protocol, error) {
	var request bytes.Buffer

	privateKey, err := encodeBase64ToHex(cfg.PrivateKey)
	if err != nil {
		slog.Error("invalid private key", "error", err)
	}

	peerPublicKey, err := encodeBase64ToHex(cfg.PeerPublicKey)
	if err != nil {
		slog.Error("invalid peer public key", "error", err)
	}

	_, err = request.WriteString(fmt.Sprintf(
		heredoc.Doc(`
			private_key=%s
			public_key=%s
			endpoint=%s
			persistent_keepalive_interval=5
			allowed_ip=0.0.0.0/0
			allowed_ip=::0/0
		`),
		privateKey, peerPublicKey, cfg.Endpoint,
	))
	if err != nil {
		return nil, err
	}

	localAddress, err := netip.ParseAddr(cfg.Address)
	if err != nil {
		return nil, err
	}

	dnss := make([]netip.Addr, 0, len(cfg.DNS))

	for i := range cfg.DNS {
		addr, err := netip.ParseAddr(cfg.DNS[i])
		if err != nil {
			return nil, err
		}

		dnss = append(dnss, addr)
	}

	slog.Debug("create tun", "ip", localAddress.String(), "mtu", strconv.Itoa(defaultMTU))

	tun, tnet, err := netstack.CreateNetTUN([]netip.Addr{localAddress}, dnss, defaultMTU)
	if err != nil {
		return nil, err
	}

	wglog := device.Logger{
		Verbosef: func(format string, args ...any) {
			slog.Debug(fmt.Sprintf(format, args...))
		},
		Errorf: func(format string, args ...any) {
			slog.Error(fmt.Sprintf(format, args...))
		},
	}

	slog.Debug("create device", "ip", localAddress.String(), "mtu", strconv.Itoa(defaultMTU))

	dev := device.NewDevice(tun, wgconn.NewDefaultBind(), &wglog)

	err = dev.IpcSet(request.String())
	if err != nil {
		return nil, err
	}

	slog.Debug("up device", "ip", localAddress.String(), "mtu", strconv.Itoa(defaultMTU))

	err = dev.Up()
	if err != nil {
		return nil, err
	}

	go func() {
		<-ctx.Done()

		dev.Close()
	}()

	return &Protocol{
		domains: cfg.Domains,
		tnet:    tnet,
		dns:     cfg.DNS,
		ips:     cfg.IPs,
	}, nil
}

func encodeBase64ToHex(key string) (string, error) {
	decoded, err := base64.StdEncoding.DecodeString(key)
	if err != nil {
		return "", fmt.Errorf("invalid base64 string (%s): %w", key, err)
	}

	if len(decoded) != 32 {
		return "", fmt.Errorf("key should be 32 bytes (%s): %w", key, errKeyInvalid)
	}

	return hex.EncodeToString(decoded), nil
}

func (p *Protocol) Domains() []string {
	return p.domains
}

func (p *Protocol) FixedIPs() []string {
	return p.ips
}

func (p *Protocol) LookupHost(ctx context.Context, req *dns.Msg) *dns.Msg {
	for _, addr := range p.dns {
		dial, err := p.tnet.DialContext(ctx, "udp", addr+":53")
		if err != nil {
			slog.Error("failed to handle dns req", append([]any{"error", err}, log.DNSAttrs(req)...)...)

			continue
		}

		co := new(dns.Conn)
		co.Conn = dial

		err = co.WriteMsg(req)
		if err != nil {
			slog.Error("failed to handle dns req", append([]any{"error", err}, log.DNSAttrs(req)...)...)

			continue
		}

		rsp, err := co.ReadMsg()
		if err != nil {
			slog.Error("failed to handle dns req", append([]any{"error", err}, log.DNSAttrs(req)...)...)

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
	remoteConn, err := p.tnet.Dial(conn.LocalAddr().Network(), conn.LocalAddr().String())
	if err != nil {
		if !errors.Is(err, io.EOF) {
			slog.Warn("handle conn", "dest", conn.LocalAddr().String(), "type", conn.LocalAddr().Network(), "error", err)
		}

		return
	}

	slog.Info("handle conn", "dest", conn.LocalAddr().String(), "type", conn.LocalAddr().Network())

	network.Transfer(conn, remoteConn)
}

func (p *Protocol) HandleUDP(conn net.Conn) {
	remoteConn, err := p.tnet.Dial(conn.LocalAddr().Network(), conn.LocalAddr().String())
	if err != nil {
		if !errors.Is(err, io.EOF) {
			slog.Warn("handle conn", "dest", conn.LocalAddr().String(), "type", conn.LocalAddr().Network(), "error", err)
		}

		return
	}

	slog.Info("handle conn", "dest", conn.LocalAddr().String(), "type", conn.LocalAddr().Network())

	network.Transfer(conn, remoteConn)
}
