package browser

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestEgressProxy_ForwardsAllowedHTTPAndRejectsBlockedTargets(t *testing.T) {
	observed := make(chan *http.Request, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		observed <- request.Clone(context.Background())
		writer.Header().Set("X-Test", "forwarded")
		writer.Header().Set("Connection", "X-Remove")
		writer.Header().Set("X-Remove", "internal")
		_, _ = io.WriteString(writer, "ok")
	}))
	defer upstream.Close()

	permissive := NetworkPolicy{Strict: false}
	proxy, err := startEgressProxy(permissive)
	require.NoError(t, err)
	proxyURL, err := url.Parse(proxy.URL())
	require.NoError(t, err)
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, upstream.URL, nil)
	require.NoError(t, err)
	response, err := client.Do(request)
	require.NoError(t, err)
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.Equal(t, "forwarded", response.Header.Get("X-Test"))
	require.Empty(t, response.Header.Get("X-Remove"))
	require.Equal(t, "ok", string(body))
	forwarded := <-observed
	require.Equal(t, request.URL.Host, forwarded.Host)
	require.Empty(t, forwarded.Header.Get("Proxy-Authorization"))
	proxy.setPolicy(NetworkPolicy{Strict: true})
	response, err = client.Do(request)
	require.NoError(t, err)
	require.Equal(t, http.StatusForbidden, response.StatusCode)
	require.NoError(t, response.Body.Close())
	unauthorizedURL := *proxyURL
	unauthorizedURL.User = nil
	unauthorizedClient := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(&unauthorizedURL)}}
	response, err = unauthorizedClient.Get(upstream.URL)
	require.NoError(t, err)
	require.Equal(t, http.StatusProxyAuthRequired, response.StatusCode)
	require.NoError(t, response.Body.Close())
	require.NoError(t, proxy.Close(context.Background()))
	require.NoError(t, proxy.Close(context.Background()))

	blocked, err := startEgressProxy(NetworkPolicy{Strict: true})
	require.NoError(t, err)
	blockedURL, err := url.Parse(blocked.URL())
	require.NoError(t, err)
	blockedClient := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(blockedURL)}}
	response, err = blockedClient.Get(upstream.URL)
	require.NoError(t, err)
	require.Equal(t, http.StatusForbidden, response.StatusCode)
	require.NoError(t, response.Body.Close())
	require.NoError(t, blocked.Close(context.Background()))
}

func TestEgressProxy_RejectsMalformedAndBlockedConnectTargets(t *testing.T) {
	proxy, err := startEgressProxy(NetworkPolicy{Strict: true})
	require.NoError(t, err)
	tests := []struct {
		target string
		status int
	}{
		{target: "bad:port", status: http.StatusBadRequest},
		{target: "127.0.0.1:443", status: http.StatusForbidden},
	}
	for _, test := range tests {
		connection, dialErr := net.Dial("tcp", proxy.listener.Addr().String())
		require.NoError(t, dialErr)
		_, dialErr = fmt.Fprintf(
			connection, "CONNECT %s HTTP/1.1\r\nHost: %s\r\nProxy-Authorization: %s\r\n\r\n",
			test.target, test.target, proxy.authorization.header(),
		)
		require.NoError(t, dialErr)
		response, readErr := http.ReadResponse(bufio.NewReader(connection), &http.Request{Method: http.MethodConnect})
		require.NoError(t, readErr)
		require.Equal(t, test.status, response.StatusCode)
		require.NoError(t, response.Body.Close())
		require.NoError(t, connection.Close())
	}
	require.NoError(t, proxy.Close(context.Background()))
}

func TestEgressProxy_BlocksWebSocketUpgrades(t *testing.T) {
	proxy := &egressProxy{policy: NetworkPolicy{Strict: false}}
	request := httptest.NewRequest(http.MethodGet, "http://example.com/socket", nil)
	request.Header.Set("Connection", "Upgrade")
	request.Header.Set("Upgrade", "websocket")
	recorder := httptest.NewRecorder()

	proxy.handleHTTP(recorder, request)

	require.Equal(t, http.StatusForbidden, recorder.Code)
}

func TestEgressProxy_TunnelsAllowedConnectTarget(t *testing.T) {
	echo, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, echo.Close()) })
	go func() {
		connection, acceptErr := echo.Accept()
		if acceptErr != nil {
			return
		}
		defer func() { _ = connection.Close() }()
		_, _ = io.Copy(connection, connection)
	}()

	proxy, err := startEgressProxy(NetworkPolicy{Strict: false})
	require.NoError(t, err)
	connection, err := net.Dial("tcp", proxy.listener.Addr().String())
	require.NoError(t, err)
	defer func() { require.NoError(t, connection.Close()) }()
	_, err = fmt.Fprintf(
		connection,
		"CONNECT %s HTTP/1.1\r\nHost: %s\r\nProxy-Authorization: %s\r\n\r\nping",
		echo.Addr(), echo.Addr(), proxy.authorization.header(),
	)
	require.NoError(t, err)
	reader := bufio.NewReader(connection)
	status, err := reader.ReadString('\n')
	require.NoError(t, err)
	require.Contains(t, status, "200")
	for {
		line, readErr := reader.ReadString('\n')
		require.NoError(t, readErr)
		if line == "\r\n" {
			break
		}
	}
	response := make([]byte, 4)
	_, err = io.ReadFull(reader, response)
	require.NoError(t, err)
	require.Equal(t, "ping", string(response))
	_, err = connection.Write([]byte("pong"))
	require.NoError(t, err)
	_, err = io.ReadFull(reader, response)
	require.NoError(t, err)
	require.Equal(t, "pong", string(response))
	require.NoError(t, proxy.closeConnections())
	require.NoError(t, connection.SetReadDeadline(time.Now().Add(time.Second)))
	_, err = reader.ReadByte()
	require.Error(t, err)
	require.Eventually(t, func() bool {
		proxy.mu.Lock()
		defer proxy.mu.Unlock()
		return len(proxy.connections) == 0
	}, time.Second, 10*time.Millisecond)
	require.NoError(t, proxy.Close(context.Background()))
}

func TestSplitProxyAddress_DefaultsAndRejectsInvalidPorts(t *testing.T) {
	host, port, err := splitProxyAddress("example.com", 443)
	require.NoError(t, err)
	require.Equal(t, "example.com", host)
	require.Equal(t, uint16(443), port)
	host, port, err = splitProxyAddress("example.com:8443", 443)
	require.NoError(t, err)
	require.Equal(t, "example.com", host)
	require.Equal(t, uint16(8443), port)
	_, _, err = splitProxyAddress("example.com:invalid", 443)
	require.Error(t, err)
	require.Empty(t, (&egressProxy{}).URL())
	require.NoError(t, (*egressProxy)(nil).Close(context.Background()))
	require.NoError(t, (&egressProxy{}).Close(context.Background()))
}

func TestRemoveProxyHopHeaders_RemovesNamedAndStandardHeaders(t *testing.T) {
	header := http.Header{
		"Connection":          []string{"X-Internal, keep-alive"},
		"X-Internal":          []string{"secret"},
		"Proxy-Authorization": []string{"secret"},
		"Upgrade":             []string{"websocket"},
		"X-Keep":              []string{"value"},
	}
	removeProxyHopHeaders(header)
	require.Equal(t, http.Header{"X-Keep": []string{"value"}}, header)
}
