package xarb

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"

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
	h := &Handler{Store: NewStore(), StatPost: func(_ context.Context, _ http.Header, b []byte) error {
		posts = append(posts, b)
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
	udp := append(hdr(1), byte(2*len(proc(1))), 0, 0, 0)
	udp = append(append(udp, proc(0xe000000000000002)...), proc(0xe000000000000001)...)
	if c, _ := do("/XArb/ReportResult.ashx", "000000000002@xbox.com", "", "xon/16", udp); c != 200 || len(posts) != 1 {
		t.Fatalf("duplicate report posted: %d %d", c, len(posts))
	}
	if c, _ := do("/XArb/ReportResult.ashx", "000000000001@xbox.com", "", "xon/5", xrl[:len(xrl)-5]); c != 400 {
		t.Fatalf("truncated report: %d", c)
	}
}
