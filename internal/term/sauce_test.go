package term

import (
	"encoding/binary"
	"testing"
)

func TestParseSAUCE(t *testing.T) {
	base := []byte("ANSI ART CONTENT")
	record := make([]byte, sauceRecordSize)
	copy(record[0:7], []byte("SAUCE00"))
	copy(record[7:42], []byte("WOLF ANSI TITLE"))
	copy(record[42:62], []byte("SEAN"))
	copy(record[62:82], []byte("WOLFBBS"))
	copy(record[82:90], []byte("20260224"))
	binary.LittleEndian.PutUint32(record[90:94], uint32(len(base)))
	record[94] = 1
	record[95] = 1
	binary.LittleEndian.PutUint16(record[96:98], 80)
	binary.LittleEndian.PutUint16(record[98:100], 25)
	record[104] = 0
	record[105] = 1
	copy(record[106:128], []byte("CP437"))

	data := append(base, record...)
	meta, ok := ParseSAUCE(data)
	if !ok {
		t.Fatal("expected SAUCE metadata")
	}
	if meta.Title != "WOLF ANSI TITLE" {
		t.Fatalf("unexpected title: %q", meta.Title)
	}
	if meta.Author != "SEAN" {
		t.Fatalf("unexpected author: %q", meta.Author)
	}
	if meta.TInfo1 != 80 || meta.TInfo2 != 25 {
		t.Fatalf("unexpected dimensions: %d x %d", meta.TInfo1, meta.TInfo2)
	}
}

func TestStripSAUCE(t *testing.T) {
	base := []byte("BODY")
	record := make([]byte, sauceRecordSize)
	copy(record[0:7], []byte("SAUCE00"))
	data := append(base, record...)
	stripped := StripSAUCE(data)
	if string(stripped) != "BODY" {
		t.Fatalf("expected body without sauce, got %q", string(stripped))
	}
}
