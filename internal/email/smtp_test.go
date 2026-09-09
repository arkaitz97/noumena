package email

import (
	"net"
	"strings"
	"testing"
	"time"
)

// TestSMTPCmdTimeoutBoundsStalledRelay verifies task 3.1/3.3: a relay
// that accepts the connection but stalls mid-command fails the send
// within the configured per-command deadline instead of hanging forever.
func TestSMTPCmdTimeoutBoundsStalledRelay(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	// Relay: greet, then never answer any command. The client's first
	// command (MAIL, since AllowInsecure skips STARTTLS) hits the
	// deadline.
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			defer conn.Close()
			_, _ = conn.Write([]byte("220 stall-relay ESMTP\r\n"))
			// Hold the connection open, never responding.
			buf := make([]byte, 1024)
			for {
				if _, err := conn.Read(buf); err != nil {
					return
				}
			}
		}
	}()

	s := &NetSMTPSender{
		Host:          "127.0.0.1",
		Port:          ln.Addr().(*net.TCPAddr).Port,
		CmdTimeout:    300 * time.Millisecond,
		AllowInsecure: true,
	}

	start := time.Now()
	err = s.Send(t.Context(), "from@example.com", "to@example.com",
		"subject", "<p>hi</p>", "hi", nil)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error from stalled relay, got nil")
	}
	if !strings.Contains(err.Error(), "smtp MAIL FROM") {
		t.Errorf("expected MAIL FROM error, got: %v", err)
	}
	if elapsed < 300*time.Millisecond {
		t.Errorf("failed too fast (deadline not enforced?): %v", elapsed)
	}
	if elapsed > 5*time.Second {
		t.Errorf("failed too slow (deadline not enforced?): %v", elapsed)
	}
}

// List-Unsubscribe header lands in the MIME payload when supplied.
func TestBuildMIMEMessageIncludesHeaders(t *testing.T) {
	msg := buildMIMEMessage("from@example.com", "to@example.com", "subj",
		"<p>html</p>", "text", []string{"List-Unsubscribe: <https://app.example/unsubscribe?sig=x>"})
	if !strings.Contains(msg, "List-Unsubscribe: <https://app.example/unsubscribe?sig=x>") {
		t.Errorf("MIME missing List-Unsubscribe header:\n%s", msg)
	}
	if !strings.Contains(msg, "multipart/alternative") {
		t.Errorf("MIME missing multipart/alternative:\n%s", msg)
	}
}
