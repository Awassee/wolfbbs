package loginserver

import (
	"strings"
	"testing"
	"time"

	"wolfbbs/internal/session"
)

type capturePeer struct {
	lines []string
}

func (p *capturePeer) ReadLine() (string, error) { return "", nil }
func (p *capturePeer) WriteLine(line string) error {
	p.lines = append(p.lines, line)
	return nil
}
func (p *capturePeer) RemoteAddr() string { return "" }
func (p *capturePeer) Close() error       { return nil }

func TestWriteWhoOnlineIncludesOriginAndHost(t *testing.T) {
	peer := &capturePeer{}
	writeWhoOnline(peer, []session.NodeState{{
		NodeID:      3,
		Username:    "sysop",
		Area:        "Main Menu",
		RemoteAddr:  "192.168.1.25:2222",
		IdleSeconds: 7,
	}})
	output := strings.Join(peer.lines, "\n")
	for _, want := range []string{"Who's Online", "From: 192.168.1.25 (LAN)", "Node 3 | sysop"} {
		if !strings.Contains(output, want) {
			t.Fatalf("who output missing %q in %q", want, output)
		}
	}
}

func TestWriteLastCallersIncludesOriginAndHost(t *testing.T) {
	peer := &capturePeer{}
	writeLastCallers(peer, []session.CallerState{{
		NodeID:     9,
		Username:   "alpha",
		Area:       "Boards",
		RemoteAddr: "bbs.example.org:2023",
		Duration:   3*time.Minute + 5*time.Second,
	}})
	output := strings.Join(peer.lines, "\n")
	for _, want := range []string{"Last Callers", "From: bbs.example.org (HOST)", "Duration: 3m5s"} {
		if !strings.Contains(output, want) {
			t.Fatalf("last callers output missing %q in %q", want, output)
		}
	}
}
