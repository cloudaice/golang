// Copyright 2009 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package net

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"reflect"
	"runtime"
	"testing"
	"time"
)

type resolveIPAddrTest struct {
	net           string
	litAddrOrName string
	addr          *IPAddr
	err           error
}

var resolveIPAddrTests = []resolveIPAddrTest{
	{"ip", "127.0.0.1", &IPAddr{IP: IPv4(127, 0, 0, 1)}, nil},
	{"ip4", "127.0.0.1", &IPAddr{IP: IPv4(127, 0, 0, 1)}, nil},
	{"ip4:icmp", "127.0.0.1", &IPAddr{IP: IPv4(127, 0, 0, 1)}, nil},

	{"ip", "::1", &IPAddr{IP: ParseIP("::1")}, nil},
	{"ip6", "::1", &IPAddr{IP: ParseIP("::1")}, nil},
	{"ip6:ipv6-icmp", "::1", &IPAddr{IP: ParseIP("::1")}, nil},
	{"ip6:IPv6-ICMP", "::1", &IPAddr{IP: ParseIP("::1")}, nil},

	{"ip", "::1%en0", &IPAddr{IP: ParseIP("::1"), Zone: "en0"}, nil},
	{"ip6", "::1%911", &IPAddr{IP: ParseIP("::1"), Zone: "911"}, nil},

	{"", "127.0.0.1", &IPAddr{IP: IPv4(127, 0, 0, 1)}, nil}, // Go 1.0 behavior
	{"", "::1", &IPAddr{IP: ParseIP("::1")}, nil},           // Go 1.0 behavior

	{"l2tp", "127.0.0.1", nil, UnknownNetworkError("l2tp")},
	{"l2tp:gre", "127.0.0.1", nil, UnknownNetworkError("l2tp:gre")},
	{"tcp", "1.2.3.4:123", nil, UnknownNetworkError("tcp")},
}

func init() {
	if ifi := loopbackInterface(); ifi != nil {
		index := fmt.Sprintf("%v", ifi.Index)
		resolveIPAddrTests = append(resolveIPAddrTests, []resolveIPAddrTest{
			{"ip6", "fe80::1%" + ifi.Name, &IPAddr{IP: ParseIP("fe80::1"), Zone: zoneToString(ifi.Index)}, nil},
			{"ip6", "fe80::1%" + index, &IPAddr{IP: ParseIP("fe80::1"), Zone: index}, nil},
		}...)
	}
	if ips, err := LookupIP("localhost"); err == nil && len(ips) > 1 && supportsIPv4 && supportsIPv6 {
		resolveIPAddrTests = append(resolveIPAddrTests, []resolveIPAddrTest{
			{"ip", "localhost", &IPAddr{IP: IPv4(127, 0, 0, 1).To4()}, nil},
			{"ip4", "localhost", &IPAddr{IP: IPv4(127, 0, 0, 1).To4()}, nil},
			{"ip6", "localhost", &IPAddr{IP: IPv6loopback}, nil},
		}...)
	}
}

func TestResolveIPAddr(t *testing.T) {
	for _, tt := range resolveIPAddrTests {
		addr, err := ResolveIPAddr(tt.net, tt.litAddrOrName)
		if err != tt.err {
			t.Fatalf("ResolveIPAddr(%v, %v) failed: %v", tt.net, tt.litAddrOrName, err)
		} else if !reflect.DeepEqual(addr, tt.addr) {
			t.Fatalf("got %#v; expected %#v", addr, tt.addr)
		}
	}
}

var icmpEchoTests = []struct {
	net   string
	laddr string
	raddr string
}{
	{"ip4:icmp", "0.0.0.0", "127.0.0.1"},
	{"ip6:ipv6-icmp", "::", "::1"},
}

func TestConnICMPEcho(t *testing.T) {
	switch runtime.GOOS {
	case "plan9":
		t.Skipf("skipping test on %q", runtime.GOOS)
	case "windows":
	default:
		if os.Getuid() != 0 {
			t.Skip("skipping test; must be root")
		}
	}

	for i, tt := range icmpEchoTests {
		net, _, err := parseNetwork(tt.net)
		if err != nil {
			t.Fatalf("parseNetwork failed: %v", err)
		}
		if net == "ip6" && !supportsIPv6 {
			continue
		}

		c, err := Dial(tt.net, tt.raddr)
		if err != nil {
			t.Fatalf("Dial failed: %v", err)
		}
		c.SetDeadline(time.Now().Add(100 * time.Millisecond))
		defer c.Close()

		typ := icmpv4EchoRequest
		if net == "ip6" {
			typ = icmpv6EchoRequest
		}
		xid, xseq := os.Getpid()&0xffff, i+1
		wb, err := (&icmpMessage{
			Type: typ, Code: 0,
			Body: &icmpEcho{
				ID: xid, Seq: xseq,
				Data: bytes.Repeat([]byte("Go Go Gadget Ping!!!"), 3),
			},
		}).Marshal()
		if err != nil {
			t.Fatalf("icmpMessage.Marshal failed: %v", err)
		}
		if _, err := c.Write(wb); err != nil {
			t.Fatalf("Conn.Write failed: %v", err)
		}
		var m *icmpMessage
		rb := make([]byte, 20+len(wb))
		for {
			if _, err := c.Read(rb); err != nil {
				t.Fatalf("Conn.Read failed: %v", err)
			}
			if net == "ip4" {
				rb = ipv4Payload(rb)
			}
			if m, err = parseICMPMessage(rb); err != nil {
				t.Fatalf("parseICMPMessage failed: %v", err)
			}
			switch m.Type {
			case icmpv4EchoRequest, icmpv6EchoRequest:
				continue
			}
			break
		}
		switch p := m.Body.(type) {
		case *icmpEcho:
			if p.ID != xid || p.Seq != xseq {
				t.Fatalf("got id=%v, seqnum=%v; expected id=%v, seqnum=%v", p.ID, p.Seq, xid, xseq)
			}
		default:
			t.Fatalf("got type=%v, code=%v; expected type=%v, code=%v", m.Type, m.Code, typ, 0)
		}
	}
}

func TestPacketConnICMPEcho(t *testing.T) {
	switch runtime.GOOS {
	case "plan9":
		t.Skipf("skipping test on %q", runtime.GOOS)
	case "windows":
	default:
		if os.Getuid() != 0 {
			t.Skip("skipping test; must be root")
		}
	}

	for i, tt := range icmpEchoTests {
		net, _, err := parseNetwork(tt.net)
		if err != nil {
			t.Fatalf("parseNetwork failed: %v", err)
		}
		if net == "ip6" && !supportsIPv6 {
			continue
		}

		c, err := ListenPacket(tt.net, tt.laddr)
		if err != nil {
			t.Fatalf("ListenPacket failed: %v", err)
		}
		c.SetDeadline(time.Now().Add(100 * time.Millisecond))
		defer c.Close()

		ra, err := ResolveIPAddr(tt.net, tt.raddr)
		if err != nil {
			t.Fatalf("ResolveIPAddr failed: %v", err)
		}
		typ := icmpv4EchoRequest
		if net == "ip6" {
			typ = icmpv6EchoRequest
		}
		xid, xseq := os.Getpid()&0xffff, i+1
		wb, err := (&icmpMessage{
			Type: typ, Code: 0,
			Body: &icmpEcho{
				ID: xid, Seq: xseq,
				Data: bytes.Repeat([]byte("Go Go Gadget Ping!!!"), 3),
			},
		}).Marshal()
		if err != nil {
			t.Fatalf("icmpMessage.Marshal failed: %v", err)
		}
		if _, err := c.WriteTo(wb, ra); err != nil {
			t.Fatalf("PacketConn.WriteTo failed: %v", err)
		}
		var m *icmpMessage
		rb := make([]byte, 20+len(wb))
		for {
			if _, _, err := c.ReadFrom(rb); err != nil {
				t.Fatalf("PacketConn.ReadFrom failed: %v", err)
			}
			// See BUG section.
			//if net == "ip4" {
			//	rb = ipv4Payload(rb)
			//}
			if m, err = parseICMPMessage(rb); err != nil {
				t.Fatalf("parseICMPMessage failed: %v", err)
			}
			switch m.Type {
			case icmpv4EchoRequest, icmpv6EchoRequest:
				continue
			}
			break
		}
		switch p := m.Body.(type) {
		case *icmpEcho:
			if p.ID != xid || p.Seq != xseq {
				t.Fatalf("got id=%v, seqnum=%v; expected id=%v, seqnum=%v", p.ID, p.Seq, xid, xseq)
			}
		default:
			t.Fatalf("got type=%v, code=%v; expected type=%v, code=%v", m.Type, m.Code, typ, 0)
		}
	}
}

func ipv4Payload(b []byte) []byte {
	if len(b) < 20 {
		return b
	}
	hdrlen := int(b[0]&0x0f) << 2
	return b[hdrlen:]
}

const (
	icmpv4EchoRequest = 8
	icmpv4EchoReply   = 0
	icmpv6EchoRequest = 128
	icmpv6EchoReply   = 129
)

// icmpMessage represents an ICMP message.
type icmpMessage struct {
	Type     int             // type
	Code     int             // code
	Checksum int             // checksum
	Body     icmpMessageBody // body
}

// icmpMessageBody represents an ICMP message body.
type icmpMessageBody interface {
	Len() int
	Marshal() ([]byte, error)
}

// Marshal returns the binary enconding of the ICMP echo request or
// reply message m.
func (m *icmpMessage) Marshal() ([]byte, error) {
	b := []byte{byte(m.Type), byte(m.Code), 0, 0}
	if m.Body != nil && m.Body.Len() != 0 {
		mb, err := m.Body.Marshal()
		if err != nil {
			return nil, err
		}
		b = append(b, mb...)
	}
	switch m.Type {
	case icmpv6EchoRequest, icmpv6EchoReply:
		return b, nil
	}
	csumcv := len(b) - 1 // checksum coverage
	s := uint32(0)
	for i := 0; i < csumcv; i += 2 {
		s += uint32(b[i+1])<<8 | uint32(b[i])
	}
	if csumcv&1 == 0 {
		s += uint32(b[csumcv])
	}
	s = s>>16 + s&0xffff
	s = s + s>>16
	// Place checksum back in header; using ^= avoids the
	// assumption the checksum bytes are zero.
	b[2] ^= byte(^s)
	b[3] ^= byte(^s >> 8)
	return b, nil
}

// parseICMPMessage parses b as an ICMP message.
func parseICMPMessage(b []byte) (*icmpMessage, error) {
	msglen := len(b)
	if msglen < 4 {
		return nil, errors.New("message too short")
	}
	m := &icmpMessage{Type: int(b[0]), Code: int(b[1]), Checksum: int(b[2])<<8 | int(b[3])}
	if msglen > 4 {
		var err error
		switch m.Type {
		case icmpv4EchoRequest, icmpv4EchoReply, icmpv6EchoRequest, icmpv6EchoReply:
			m.Body, err = parseICMPEcho(b[4:])
			if err != nil {
				return nil, err
			}
		}
	}
	return m, nil
}

// imcpEcho represenets an ICMP echo request or reply message body.
type icmpEcho struct {
	ID   int    // identifier
	Seq  int    // sequence number
	Data []byte // data
}

func (p *icmpEcho) Len() int {
	if p == nil {
		return 0
	}
	return 4 + len(p.Data)
}

// Marshal returns the binary enconding of the ICMP echo request or
// reply message body p.
func (p *icmpEcho) Marshal() ([]byte, error) {
	b := make([]byte, 4+len(p.Data))
	b[0], b[1] = byte(p.ID>>8), byte(p.ID)
	b[2], b[3] = byte(p.Seq>>8), byte(p.Seq)
	copy(b[4:], p.Data)
	return b, nil
}

// parseICMPEcho parses b as an ICMP echo request or reply message
// body.
func parseICMPEcho(b []byte) (*icmpEcho, error) {
	bodylen := len(b)
	p := &icmpEcho{ID: int(b[0])<<8 | int(b[1]), Seq: int(b[2])<<8 | int(b[3])}
	if bodylen > 4 {
		p.Data = make([]byte, bodylen-4)
		copy(p.Data, b[4:])
	}
	return p, nil
}

var ipConnLocalNameTests = []struct {
	net   string
	laddr *IPAddr
}{
	{"ip4:icmp", &IPAddr{IP: IPv4(127, 0, 0, 1)}},
	{"ip4:icmp", &IPAddr{}},
	{"ip4:icmp", nil},
}

func TestIPConnLocalName(t *testing.T) {
	switch runtime.GOOS {
	case "plan9", "windows":
		t.Skipf("skipping test on %q", runtime.GOOS)
	default:
		if os.Getuid() != 0 {
			t.Skip("skipping test; must be root")
		}
	}

	for _, tt := range ipConnLocalNameTests {
		c, err := ListenIP(tt.net, tt.laddr)
		if err != nil {
			t.Fatalf("ListenIP failed: %v", err)
		}
		defer c.Close()
		if la := c.LocalAddr(); la == nil {
			t.Fatal("IPConn.LocalAddr failed")
		}
	}
}

func TestIPConnRemoteName(t *testing.T) {
	switch runtime.GOOS {
	case "plan9", "windows":
		t.Skipf("skipping test on %q", runtime.GOOS)
	default:
		if os.Getuid() != 0 {
			t.Skip("skipping test; must be root")
		}
	}

	raddr := &IPAddr{IP: IPv4(127, 0, 0, 10).To4()}
	c, err := DialIP("ip:tcp", &IPAddr{IP: IPv4(127, 0, 0, 1)}, raddr)
	if err != nil {
		t.Fatalf("DialIP failed: %v", err)
	}
	defer c.Close()
	if !reflect.DeepEqual(raddr, c.RemoteAddr()) {
		t.Fatalf("got %#v, expected %#v", c.RemoteAddr(), raddr)
	}
}
