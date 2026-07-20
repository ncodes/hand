package browser

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	stdlog "log"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/wandxy/morph/internal/permissions"
)

const defaultProxyReadHeaderTimeout = 10 * time.Second

type egressProxy struct {
	authorization localAuthorization
	listener      net.Listener
	server        *http.Server
	policyMu      sync.RWMutex
	policy        NetworkPolicy
	mu            sync.Mutex
	closed        bool
	connections   map[net.Conn]struct{}
}

func startEgressProxy(policy NetworkPolicy) (*egressProxy, error) {
	authorization, err := newLocalAuthorization()
	if err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	proxy := &egressProxy{
		authorization: authorization, policy: policy, listener: listener, connections: make(map[net.Conn]struct{}),
	}
	proxy.server = &http.Server{
		Handler:           http.HandlerFunc(proxy.handle),
		ReadHeaderTimeout: defaultProxyReadHeaderTimeout,
		ErrorLog:          stdlog.New(io.Discard, "", 0),
	}
	go func() {
		_ = proxy.server.Serve(listener)
	}()

	return proxy, nil
}

func (p *egressProxy) URL() string {
	if p == nil || p.listener == nil {
		return ""
	}

	return (&url.URL{
		Scheme: "http",
		User:   p.authorization.userinfo(),
		Host:   p.listener.Addr().String(),
	}).String()
}

func (p *egressProxy) chromiumURL() string {
	if p == nil || p.listener == nil {
		return ""
	}

	return (&url.URL{Scheme: "http", Host: p.listener.Addr().String()}).String()
}

func (p *egressProxy) setPolicy(policy NetworkPolicy) {
	if p == nil {
		return
	}
	p.policyMu.Lock()
	p.policy = policy
	p.policyMu.Unlock()
}

func (p *egressProxy) getPolicy() NetworkPolicy {
	p.policyMu.RLock()
	defer p.policyMu.RUnlock()

	return p.policy
}

func (p *egressProxy) Close(ctx context.Context) error {
	if p == nil || p.server == nil {
		return nil
	}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	p.mu.Unlock()

	shutdownErr := p.server.Shutdown(ctx)
	if shutdownErr != nil {
		shutdownErr = errors.Join(shutdownErr, p.server.Close())
	}
	return errors.Join(shutdownErr, p.closeConnections())
}

func (p *egressProxy) closeConnections() error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	connections := make([]net.Conn, 0, len(p.connections))
	for connection := range p.connections {
		connections = append(connections, connection)
	}
	p.mu.Unlock()
	var closeErrors []error
	for _, connection := range connections {
		if err := connection.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			closeErrors = append(closeErrors, err)
		}
	}

	return errors.Join(closeErrors...)
}

func (p *egressProxy) handle(writer http.ResponseWriter, request *http.Request) {
	if !p.authorization.requireProxy(writer, request) {
		return
	}
	if request.Method == http.MethodConnect {
		p.handleConnect(writer, request)
		return
	}
	p.handleHTTP(writer, request)
}

func (p *egressProxy) handleConnect(writer http.ResponseWriter, request *http.Request) {
	host, port, err := splitProxyAddress(request.Host, 443)
	if err != nil {
		log.Warn().
			Str("browser_network_stage", "proxy_connect_validation").
			Str("network_method", http.MethodConnect).
			Str("error", getSafeBrowserNetworkError(err)).
			Msg("Browser proxy rejected an invalid CONNECT target")
		http.Error(writer, "invalid proxy target", http.StatusBadRequest)
		return
	}
	target := permissions.NetworkTarget{
		Scheme: "https", Host: host, Port: port, Path: "/", Method: http.MethodConnect,
		RequestClass: permissions.NetworkRequestSubresource,
	}
	upstream, err := p.dialTarget(request.Context(), target)
	if err != nil {
		addBrowserNetworkLogFields(log.Warn(), target).
			Str("browser_network_stage", "proxy_connect").
			Str("error", getSafeBrowserNetworkError(err)).
			Msg("Browser proxy blocked or failed to connect to an upstream target")
		http.Error(writer, "blocked proxy target", http.StatusForbidden)
		return
	}
	hijacker, ok := writer.(http.Hijacker)
	if !ok {
		_ = upstream.Close()
		http.Error(writer, "proxy transport unavailable", http.StatusInternalServerError)
		return
	}
	client, buffer, err := hijacker.Hijack()
	if err != nil {
		_ = upstream.Close()
		return
	}
	_, _ = buffer.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n")
	if err := buffer.Flush(); err != nil {
		addBrowserNetworkLogFields(log.Warn(), target).
			Str("browser_network_stage", "proxy_connect_response").
			Str("error", getSafeBrowserNetworkError(err)).
			Msg("Browser proxy failed to establish a CONNECT tunnel")
		_ = client.Close()
		_ = upstream.Close()
		return
	}
	if buffered := buffer.Reader.Buffered(); buffered > 0 {
		if _, err := io.CopyN(upstream, buffer.Reader, int64(buffered)); err != nil {
			_ = client.Close()
			_ = upstream.Close()
			return
		}
	}
	if !p.trackConnections(client, upstream) {
		_ = client.Close()
		_ = upstream.Close()
		return
	}
	addBrowserNetworkLogFields(log.Debug(), target).
		Str("browser_network_stage", "proxy_connect").
		Msg("Browser proxy established an upstream tunnel")
	go func() {
		proxyTunnel(client, upstream)
		p.untrackConnections(client, upstream)
	}()
}

func (p *egressProxy) handleHTTP(writer http.ResponseWriter, request *http.Request) {
	if strings.EqualFold(strings.TrimSpace(request.Header.Get("Upgrade")), "websocket") {
		log.Warn().
			Str("browser_network_stage", "proxy_http_validation").
			Str("network_method", request.Method).
			Msg("Browser proxy blocked a WebSocket upgrade")
		http.Error(writer, "blocked proxy target", http.StatusForbidden)
		return
	}
	if request.URL == nil || request.URL.Hostname() == "" {
		log.Warn().
			Str("browser_network_stage", "proxy_http_validation").
			Str("network_method", request.Method).
			Msg("Browser proxy rejected an invalid HTTP target")
		http.Error(writer, "invalid proxy target", http.StatusBadRequest)
		return
	}
	target, err := permissions.NetworkTargetFromURL(
		request.URL.String(), request.Method, permissions.NetworkRequestSubresource,
	)
	if err != nil {
		log.Warn().
			Str("browser_network_stage", "proxy_http_validation").
			Str("network_method", request.Method).
			Str("error", getSafeBrowserNetworkError(err)).
			Msg("Browser proxy rejected an invalid HTTP target")
		http.Error(writer, "invalid proxy target", http.StatusBadRequest)
		return
	}
	addresses, err := p.getPolicy().Resolve(request.Context(), target)
	if err != nil {
		addBrowserNetworkLogFields(log.Warn(), target).
			Str("browser_network_stage", "proxy_policy").
			Str("error", getSafeBrowserNetworkError(err)).
			Msg("Browser proxy blocked a target by network policy")
		http.Error(writer, "blocked proxy target", http.StatusForbidden)
		return
	}
	transport := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return dialResolvedTarget(ctx, addresses, target.Port)
		},
	}
	defer transport.CloseIdleConnections()
	forward := request.Clone(request.Context())
	forward.RequestURI = ""
	forward.Host = request.URL.Host
	removeProxyHopHeaders(forward.Header)
	response, err := transport.RoundTrip(forward)
	if err != nil {
		addBrowserNetworkLogFields(log.Warn(), target).
			Str("browser_network_stage", "proxy_upstream").
			Str("error", getSafeBrowserNetworkError(err)).
			Msg("Browser proxy request to the upstream target failed")
		http.Error(writer, "proxy request failed", http.StatusBadGateway)
		return
	}
	defer func() { _ = response.Body.Close() }()
	copyProxyHeaders(writer.Header(), response.Header)
	writer.WriteHeader(response.StatusCode)
	_, _ = io.Copy(writer, response.Body)
	addBrowserNetworkLogFields(log.Debug(), target).
		Str("browser_network_stage", "proxy_upstream").
		Int("http_status", response.StatusCode).
		Msg("Browser proxy request completed")
}

func addBrowserNetworkLogFields(event *zerolog.Event, target permissions.NetworkTarget) *zerolog.Event {
	return event.
		Str("network_scheme", target.Scheme).
		Str("network_host", target.Host).
		Uint16("network_port", target.Port).
		Str("network_path", target.Path).
		Str("network_method", target.Method).
		Str("network_request_class", string(target.RequestClass)).
		Bool("network_has_query", target.QueryHash != "")
}

func getSafeBrowserNetworkError(err error) string {
	var urlError *url.Error
	if errors.As(err, &urlError) && urlError.Err != nil {
		return urlError.Err.Error()
	}
	return err.Error()
}

func (p *egressProxy) trackConnections(connections ...net.Conn) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return false
	}
	for _, connection := range connections {
		p.connections[connection] = struct{}{}
	}

	return true
}

func (p *egressProxy) untrackConnections(connections ...net.Conn) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, connection := range connections {
		delete(p.connections, connection)
	}
}

func (p *egressProxy) dialTarget(ctx context.Context, target permissions.NetworkTarget) (net.Conn, error) {
	addresses, err := p.getPolicy().Resolve(ctx, target)
	if err != nil {
		return nil, err
	}
	return dialResolvedTarget(ctx, addresses, target.Port)
}

func dialResolvedTarget(ctx context.Context, addresses []netip.Addr, port uint16) (net.Conn, error) {
	values := make([]net.IP, 0, len(addresses))
	for _, address := range addresses {
		values = append(values, net.IP(address.AsSlice()))
	}
	return dialResolvedIPs(ctx, values, port)
}

func splitProxyAddress(raw string, defaultPort uint16) (string, uint16, error) {
	host, portText, err := net.SplitHostPort(raw)
	if err != nil {
		parsed, parseErr := url.Parse("https://" + raw)
		if parseErr != nil || parsed.Hostname() == "" || parsed.Port() != "" {
			return "", 0, err
		}
		return parsed.Hostname(), defaultPort, nil
	}
	port, err := strconv.ParseUint(portText, 10, 16)
	if err != nil || port == 0 {
		return "", 0, fmt.Errorf("invalid proxy port")
	}

	return host, uint16(port), nil
}

func copyProxyHeaders(destination http.Header, source http.Header) {
	removeProxyHopHeaders(source)
	for key, values := range source {
		for _, value := range values {
			destination.Add(key, value)
		}
	}
}

func removeProxyHopHeaders(header http.Header) {
	for _, value := range header.Values("Connection") {
		for token := range strings.SplitSeq(value, ",") {
			header.Del(strings.TrimSpace(token))
		}
	}
	for _, name := range []string{
		"Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization", "Proxy-Connection", "Te",
		"Trailer", "Transfer-Encoding", "Upgrade",
	} {
		header.Del(name)
	}
}

func proxyTunnel(left, right net.Conn) {
	defer func() { _ = left.Close() }()
	defer func() { _ = right.Close() }()
	done := make(chan struct{}, 2)
	copyConnection := func(destination net.Conn, source net.Conn) {
		_, _ = io.Copy(destination, bufio.NewReader(source))
		done <- struct{}{}
	}
	go copyConnection(left, right)
	go copyConnection(right, left)
	<-done
}
