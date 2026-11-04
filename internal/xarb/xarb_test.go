package xarb

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/openpak/x360-common/pgstate"
	"github.com/openpak/x360-common/wire"
)

func hdr(flags uint16) []byte {
	b := binary.LittleEndian.AppendUint16(nil, 1)
	b = binary.LittleEndian.AppendUint16(b, flags)
	b = binary.LittleEndian.AppendUint32(b, 0x4d5307e6)
	b = binary.LittleEndian.AppendUint64(b, 0x1122334455667788)
	return binary.LittleEndian.AppendUint64(b, 0x99)
}

// proc 0x8003 (Sum) for xuid in view 3: column 1 += 5.
func proc(xuid uint64) []byte {
	b, _ := hex.DecodeString("03800500")
	b = append(b, 7)
	b = binary.LittleEndian.AppendUint64(b, xuid)
	b = append(b, 3, 3, 0, 0, 0, 2, 0, 0, 2, 1, 0, 3, 5, 0, 0, 0)
	return b
}

func TestArbitration(t *testing.T) {
	var posts [][]byte
	var postHdr http.Header
	h := &Handler{Store: NewStore(), StatPost: func(_ context.Context, hd http.Header, b []byte) error {
		posts = append(posts, b)
		postHdr = hd
		return nil
	}}
	do := func(path, acct, users, ctype string, body []byte) (int, []byte) {
		r := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
		r.Header.Set(wire.AccountHeader, acct)
		r.Header.Set(wire.UsersHeader, users)
		r.Header.Set("Content-Type", ctype)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code, w.Body.Bytes()
	}
	reg := append(hdr(6), 0x2c, 0x01)
	if c, b := do("/XArb/Register.ashx", "000000000001@xbox.com", "e000000000000001:a", "xon/5", reg); c != 200 ||
		hex.EncodeToString(b) != "0100"+"0100000000000900"+"0101"+"01000000000000e0"+"00000000" {
		t.Fatalf("register 1: %d %x", c, b)
	}
	c, b := do("/XArb/Register.ashx", "000000000002@xbox.com", "e000000000000002:b", "xon/5", reg)
	t.Logf("register reply %x", b)
	if c != 200 || b[0] != 2 || len(b) != 2+2*(10+12) {
		t.Fatalf("register 2: %d %x", c, b)
	}
	if c, _ := do("/XArb/TimeExtend.ashx", "000000000002@xbox.com", "", "xon/16", reg); c != 200 {
		t.Fatalf("extend: %d", c)
	}
	if c, _ := do("/XArb/TimeExtend.ashx", "000000000003@xbox.com", "", "xon/16", reg); c != 403 {
		t.Fatalf("extend by a stranger: %d", c)
	}
	// XRL report from console 1 with both players' rows, then the same from
	// console 2 over UDP: only the first lands.
	xrl := append(hdr(0x100), 0, 0, 0, 0, 0xe6, 0x07, 0x53, 0x4d, 2, 0)
	xrl = append(append(xrl, proc(0xe000000000000001)...), proc(0xe000000000000002)...)
	t.Logf("report body %x", xrl)
	if c, _ := do("/XArb/ReportResult.ashx", "000000000001@xbox.com", "", "xon/5", xrl); c != 200 || len(posts) != 1 ||
		len(posts[0]) != 6+2*len(proc(1)) || binary.LittleEndian.Uint32(posts[0]) != 0x4d5307e6 {
		t.Fatalf("xrl report: %d %x", c, posts)
	}
	if got := postHdr.Get(ArbUsersHeader); got != "e000000000000001,e000000000000002" {
		t.Fatalf("participants header %q", got)
	}
	udp := append(hdr(1), byte(2*len(proc(1))), 0, 0, 0)
	udp = append(append(udp, proc(0xe000000000000002)...), proc(0xe000000000000001)...)
	if c, _ := do("/XArb/ReportResult.ashx", "000000000002@xbox.com", "", "xon/16", udp); c != 200 || len(posts) != 1 {
		t.Fatalf("duplicate report posted: %d %d", c, len(posts))
	}
	if c, _ := do("/XArb/ReportResult.ashx", "000000000001@xbox.com", "", "xon/5", xrl[:len(xrl)-5]); c != 400 {
		t.Fatalf("truncated report: %d", c)
	}
}

// A result row for an XUID that never registered in the session is dropped before xstats.
func TestReportDropsNonParticipant(t *testing.T) {
	var posts [][]byte
	h := &Handler{Store: NewStore(), StatPost: func(_ context.Context, _ http.Header, b []byte) error {
		posts = append(posts, b)
		return nil
	}}
	do := func(path, acct, users, ctype string, body []byte) int {
		r := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
		r.Header.Set(wire.AccountHeader, acct)
		r.Header.Set(wire.UsersHeader, users)
		r.Header.Set("Content-Type", ctype)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}
	reg := append(hdr(6), 0x2c, 0x01)
	do("/XArb/Register.ashx", "000000000001@xbox.com", "e000000000000001:a", "xon/5", reg)
	xrl := append(hdr(0x100), 0, 0, 0, 0, 0xe6, 0x07, 0x53, 0x4d, 2, 0)
	xrl = append(append(xrl, proc(0xe000000000000001)...), proc(0xe0000000000000ff)...)
	if c := do("/XArb/ReportResult.ashx", "000000000001@xbox.com", "", "xon/5", xrl); c != 200 ||
		len(posts) != 1 || len(posts[0]) != 6+len(proc(1)) {
		t.Fatalf("report with a stranger's row: %d %x", c, posts)
	}
}

func dsn(t *testing.T) string {
	t.Helper()
	d := os.Getenv("X360_TEST_DATABASE_URL")
	if d == "" {
		t.Skip("X360_TEST_DATABASE_URL not set")
	}
	return d
}

func TestOpenStoreRefusesWithoutDSN(t *testing.T) {
	if _, err := OpenStore(context.Background(), "", Schema); err == nil {
		t.Fatal("started without a database URL")
	}
}

// TestRestartKeepsSessions: registrations and the rows already taken from a report survive a
// restart (the store reopened from Postgres), so a late duplicate report still drops.
func TestRestartKeepsSessions(t *testing.T) {
	d := dsn(t)
	b := make([]byte, 6)
	rand.Read(b)
	schema := "t_xarb_" + hex.EncodeToString(b)
	t.Cleanup(func() {
		if p, err := pgstate.Open(context.Background(), d, schema); err == nil {
			p.Pool().Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
			p.Close()
		}
	})
	var posts int
	mk := func() (*Handler, *Store) {
		s, err := OpenStore(context.Background(), d, schema)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(s.Close)
		return &Handler{Store: s, StatPost: func(context.Context, http.Header, []byte) error { posts++; return nil }}, s
	}
	do := func(h *Handler, path, acct, ctype string, body []byte) (int, []byte) {
		r := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
		r.Header.Set(wire.AccountHeader, acct)
		r.Header.Set("Content-Type", ctype)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code, w.Body.Bytes()
	}
	h, s := mk()
	reg := append(hdr(6), 0x2c, 0x01)
	if c, _ := do(h, "/XArb/Register.ashx", "000000000001@xbox.com", "xon/5", reg); c != 200 {
		t.Fatalf("register: %d", c)
	}
	xrl := append(hdr(0x100), 0, 0, 0, 0, 0xe6, 0x07, 0x53, 0x4d, 1, 0)
	xrl = append(xrl, proc(0xe000000000000001)...)
	if c, _ := do(h, "/XArb/ReportResult.ashx", "000000000001@xbox.com", "xon/5", xrl); c != 200 || posts != 1 {
		t.Fatalf("report: %d %d", c, posts)
	}
	s.Close()

	h, _ = mk() // restart
	if c, _ := do(h, "/XArb/TimeExtend.ashx", "000000000003@xbox.com", "xon/16", reg); c != 403 {
		t.Fatalf("a stranger after the restart: %d (registrations lost?)", c)
	}
	if c, _ := do(h, "/XArb/ReportResult.ashx", "000000000001@xbox.com", "xon/5", xrl); c != 200 || posts != 1 {
		t.Fatalf("duplicate after the restart posted: %d %d", c, posts)
	}
}
