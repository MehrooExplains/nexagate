//go:build linux

package app

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"syscall"
	"time"
)

func RunDNSProxy(listenAddress, upstreamAddress, device string) error {
	if _, err := net.ResolveUDPAddr("udp", upstreamAddress); err != nil {
		return fmt.Errorf("invalid upstream: %w", err)
	}
	listenerAddress, err := net.ResolveUDPAddr("udp", listenAddress)
	if err != nil {
		return err
	}
	listener, err := net.ListenUDP("udp", listenerAddress)
	if err != nil {
		return err
	}
	defer listener.Close()
	log.Printf("DNS proxy listening on %s; upstream %s is locked to %s", listenAddress, upstreamAddress, device)
	buffer := make([]byte, 4096)
	for {
		n, client, err := listener.ReadFromUDP(buffer)
		if err != nil {
			return err
		}
		if n < 12 || n > 4096 {
			continue
		}
		packet := append([]byte(nil), buffer[:n]...)
		go relayDNS(listener, client, packet, upstreamAddress, device)
	}
}

func relayDNS(listener *net.UDPConn, client *net.UDPAddr, query []byte, upstream, device string) {
	response, err := exchangeDNS("udp", query, upstream, device)
	if err != nil {
		return
	}
	// If the upstream UDP answer is truncated, retry over TCP while preserving
	// the same WARP-bound socket policy, then return the complete DNS payload.
	if len(response) >= 4 && response[2]&0x02 != 0 {
		if tcpResponse, tcpErr := exchangeDNS("tcp", query, upstream, device); tcpErr == nil {
			response = tcpResponse
		}
	}
	_, _ = listener.WriteToUDP(response, client)
}

func exchangeDNS(network string, query []byte, upstream, device string) ([]byte, error) {
	dialer := net.Dialer{Timeout: 3 * time.Second, Control: func(network, address string, raw syscall.RawConn) error {
		var setErr error
		if err := raw.Control(func(fd uintptr) {
			setErr = syscall.SetsockoptString(int(fd), syscall.SOL_SOCKET, syscall.SO_BINDTODEVICE, device)
		}); err != nil {
			return err
		}
		return setErr
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	connection, err := dialer.DialContext(ctx, network, upstream)
	if err != nil {
		return nil, err
	}
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(4 * time.Second))
	if network == "tcp" {
		if len(query) > 65535 {
			return nil, fmt.Errorf("DNS query is too large")
		}
		framed := make([]byte, len(query)+2)
		binary.BigEndian.PutUint16(framed[:2], uint16(len(query)))
		copy(framed[2:], query)
		if _, err := connection.Write(framed); err != nil {
			return nil, err
		}
		lengthPrefix := make([]byte, 2)
		if _, err := io.ReadFull(connection, lengthPrefix); err != nil {
			return nil, err
		}
		responseLength := int(binary.BigEndian.Uint16(lengthPrefix))
		if responseLength < 12 || responseLength > 65535 {
			return nil, fmt.Errorf("invalid DNS response length")
		}
		response := make([]byte, responseLength)
		if _, err := io.ReadFull(connection, response); err != nil {
			return nil, err
		}
		return response, nil
	}
	if _, err := connection.Write(query); err != nil {
		return nil, err
	}
	response := make([]byte, 4096)
	n, err := connection.Read(response)
	if err != nil {
		return nil, fmt.Errorf("read DNS response: %w", err)
	}
	if n < 12 {
		return nil, fmt.Errorf("short DNS response")
	}
	return response[:n], nil
}
