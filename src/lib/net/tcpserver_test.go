// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package net

import (
	"os";
	"io";
	"net";
	"testing";
)

func Echo(fd io.ReadWrite, done *chan<- int) {
	var buf [1024]byte;

	for {
		n, err := fd.Read(buf);
		if err != nil || n == 0 {
			break;
		}
		fd.Write(buf[0:n])
	}
	done <- 1
}

func Serve(t *testing.T, network, addr string, listening, done *chan<- int) {
	l, err := net.Listen(network, addr);
	if err != nil {
		t.Fatalf("net.Listen(%q, %q) = _, %v", network, addr, err);
	}
	listening <- 1;

	for {
		fd, addr, err := l.Accept();
		if err != nil {
			break;
		}
		echodone := new(chan int);
		go Echo(fd, echodone);
		<-echodone;	// make sure Echo stops
		l.Close();
	}
	done <- 1
}

func Connect(t *testing.T, network, addr string) {
	fd, err := net.Dial(network, "", addr);
	if err != nil {
		t.Fatalf("net.Dial(%q, %q, %q) = _, %v", network, "", addr, err);
	}

	b := io.StringBytes("hello, world\n");
	var b1 [100]byte;

	n, errno := fd.Write(b);
	if n != len(b) {
		t.Fatalf("fd.Write(%q) = %d, %v", b, n, errno);
	}

	n, errno = fd.Read(b1);
	if n != len(b) {
		t.Fatalf("fd.Read() = %d, %v", n, errno);
	}
	fd.Close();
}

func DoTest(t *testing.T, network, listenaddr, dialaddr string) {
	t.Logf("Test %s %s %s\n", network, listenaddr, dialaddr);
	listening := new(chan int);
	done := new(chan int);
	go Serve(t, network, listenaddr, listening, done);
	<-listening;	// wait for server to start
	Connect(t, network, dialaddr);
	<-done;	// make sure server stopped
}

export func TestTcpServer(t *testing.T) {
	DoTest(t,  "tcp", "0.0.0.0:9999", "127.0.0.1:9999");
	DoTest(t, "tcp", "[::]:9999", "[::ffff:127.0.0.1]:9999");
	DoTest(t, "tcp", "[::]:9999", "127.0.0.1:9999");
	DoTest(t, "tcp", "0.0.0.0:9999", "[::ffff:127.0.0.1]:9999");
}

