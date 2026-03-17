package sshserver

import (
	"bufio"
	"io"
	"strings"
	"testing"
	"time"
)

func TestReadKeyEscWithoutSequenceDoesNotBlock(t *testing.T) {
	pr, pw := io.Pipe()
	defer pw.Close()

	reader := bufio.NewReader(pr)
	done := make(chan struct {
		key string
		err error
	}, 1)

	go func() {
		key, err := readKey(reader)
		done <- struct {
			key string
			err error
		}{key: key, err: err}
	}()

	if _, err := pw.Write([]byte{0x1b}); err != nil {
		t.Fatalf("write ESC byte: %v", err)
	}

	select {
	case got := <-done:
		if got.err != nil {
			t.Fatalf("readKey returned error: %v", got.err)
		}
		if got.key != "ESC" {
			t.Fatalf("expected ESC, got %q", got.key)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatalf("readKey blocked after receiving bare ESC")
	}
}

func TestReadKeyArrowSequence(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader("\x1b[A"))
	key, err := readKey(reader)
	if err != nil {
		t.Fatalf("readKey returned error: %v", err)
	}
	if key != "UP" {
		t.Fatalf("expected UP, got %q", key)
	}
}

func TestReadKeyEscBracketWithoutSequenceDoesNotBlock(t *testing.T) {
	pr, pw := io.Pipe()
	defer pw.Close()

	reader := bufio.NewReader(pr)
	done := make(chan struct {
		key string
		err error
	}, 1)

	go func() {
		key, err := readKey(reader)
		done <- struct {
			key string
			err error
		}{key: key, err: err}
	}()

	if _, err := pw.Write([]byte{0x1b, '['}); err != nil {
		t.Fatalf("write ESC [ bytes: %v", err)
	}

	select {
	case got := <-done:
		if got.err != nil {
			t.Fatalf("readKey returned error: %v", got.err)
		}
		if got.key != "ESC" {
			t.Fatalf("expected ESC, got %q", got.key)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatalf("readKey blocked after receiving partial escape sequence")
	}
}
