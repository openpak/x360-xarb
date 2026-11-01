// Package xarb serves Xbox Live arbitration (Live service 16): the requests XAM
// sends for XSessionArbitrationRegister and for the ranked-session result and
// time-extension reports that follow. Layouts come from the XAM 17559 schemas
// and builders (docs/protocol.md).
package xarb

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/openpak/x360-common/wire"
)

const (
	maxBody         = 16 << 10
	maxRegistrants  = 32
	maxPerMachine   = 16
	maxUsers        = 4
	trustworthiness = 1
	// After the session's own lifetime ends, reports are still taken for this long.
	grace = 30 * time.Minute
	// machineIDPrefix | console digits is the machine account id the KDC issues
	// (x360-common/kdc accounts.go).
	machineIDPrefix = 0x0009000000000000
)

var errShort = errors.New("xarb: short body")

type key struct{ kid, nonce uint64 }

type registrant struct {
	machine uint64
	users   []uint64
}

type arb struct {
	title   uint32
	regs    []registrant
	expires time.Time
	written map[[2]uint64]bool // (xuid, view) rows already taken from a report
}

// Store holds the arbitrated sessions in memory.
// ponytail: a restart forgets registrations; consoles re-register for their next match.
type Store struct {
	mu  sync.Mutex
	m   map[key]*arb
	now func() time.Time
}

func NewStore() *Store { return &Store{m: map[key]*arb{}, now: time.Now} }

func (s *Store) expire() {
	for k, a := range s.m {
		if s.now().After(a.expires) {
			delete(s.m, k)
		}
	}
}

// Handler routes /XArb/*.ashx. Wrap it with wire.Guard.
type Handler struct {
	Store *Store
	Log   *slog.Logger
	// StatPost receives the stats of a result report as an xstats statpost
	// (XRL 0x705) body, with the reporting console's identity headers.
	StatPost func(ctx context.Context, hdr http.Header, body []byte) error
}

func (h *Handler) log() *slog.Logger {
	if h.Log != nil {
		return h.Log
	}
	return slog.Default()
}

type httpErr string

func (e httpErr) Error() string { return string(e) }

const (
	errNotRegistrant = httpErr("console is not a registrant of this arbitration")
	errLimit         = httpErr("too many arbitrations")
)

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.ToLower(r.URL.Path)
	if path == "/healthz" {
		return
	}
	acct := r.Header.Get(wire.AccountHeader)
	digits, _, _ := strings.Cut(acct, "@")
	n, err := strconv.ParseUint(digits, 10, 64)
	if acct == "" || err != nil || n >= 1<<48 {
		http.Error(w, "no console account", http.StatusForbidden)
		return
	}
	machine := machineIDPrefix | n
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBody+1))
	if err != nil || len(body) > maxBody || r.Method != http.MethodPost {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	var reply []byte
	switch path {
	case "/xarb/register.ashx":
		reply, err = h.register(machine, xuids(r), body)
	case "/xarb/timeextend.ashx":
		err = h.extend(machine, body)
	case "/xarb/reportresult.ashx":
		err = h.report(r, machine, body)
	default:
		h.log().Info("unhandled", "path", r.URL.Path, "body", hex.EncodeToString(body))
		http.NotFound(w, r)
		return
	}
	h.log().Info("xarb", "path", r.URL.Path, "account", acct, "content_type", r.Header.Get("Content-Type"),
		"body", hex.EncodeToString(body), "reply_len", len(reply), "err", err)
	if err != nil {
		code := http.StatusBadRequest
		if err == errNotRegistrant {
			code = http.StatusForbidden
		}
		http.Error(w, err.Error(), code)
		return
	}
	w.Header().Set("Content-Type", "xon/5")
	w.Header().Set("Content-Length", strconv.Itoa(len(reply)))
	w.Write(reply)
}

// xuids are the users signed in on the tunnel ("<xuid hex>:<account>,...").
func xuids(r *http.Request) []uint64 {
	var out []uint64
	for _, u := range strings.Split(r.Header.Get(wire.UsersHeader), ",") {
		x, _, _ := strings.Cut(strings.TrimSpace(u), ":")
		if v, err := strconv.ParseUint(x, 16, 64); err == nil && len(out) < maxUsers {
			out = append(out, v)
		}
	}
	return out
}

// header is the head every arbitration body starts with (XRL 0xF00-0xF02 and
// the UDP form): u16 version 1, u16 flags, u32 title, u64 XNKID, u64 nonce.
func header(b []byte) (flags uint16, title uint32, k key, rest []byte, err error) {
	if len(b) < 24 {
		return 0, 0, k, nil, errShort
	}
	le := binary.LittleEndian
	return le.Uint16(b[2:]), le.Uint32(b[4:]), key{le.Uint64(b[8:]), le.Uint64(b[16:])}, b[24:], nil
}

// register is XRL 0xF00 (FUN_8182d4e0, called by FUN_8191ac28): the header with
// flags 6, then u16 session lifetime in seconds. The reply lists every
// registrant so far: u16 n, n x {u64 machine id, u8 trust, u8 m, m x {u64
// xuid, u32}}. XAM keeps only the XUIDs that are members of its session.
func (h *Handler) register(machine uint64, users []uint64, body []byte) ([]byte, error) {
	_, title, k, rest, err := header(body)
	if err != nil || len(rest) < 2 {
		return nil, errShort
	}
	life := time.Duration(binary.LittleEndian.Uint16(rest)) * time.Second
	s := h.Store
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expire()
	a := s.m[k]
	if a == nil {
		n := 0
		for _, x := range s.m {
			for _, r := range x.regs {
				if r.machine == machine {
					n++
				}
			}
		}
		if n >= maxPerMachine || len(s.m) >= 1<<16 {
			return nil, errLimit
		}
		a = &arb{title: title, written: map[[2]uint64]bool{}}
		s.m[k] = a
	}
	if t := s.now().Add(life + grace); t.After(a.expires) {
		a.expires = t
	}
	found := false
	for i := range a.regs {
		if a.regs[i].machine == machine {
			a.regs[i].users, found = users, true
		}
	}
	if !found {
		if len(a.regs) >= maxRegistrants {
			return nil, errLimit
		}
		a.regs = append(a.regs, registrant{machine, users})
	}
	w := binary.LittleEndian.AppendUint16(nil, uint16(len(a.regs)))
	for _, r := range a.regs {
		w = binary.LittleEndian.AppendUint64(w, r.machine)
		w = append(w, trustworthiness, byte(len(r.users)))
		for _, u := range r.users {
			w = binary.LittleEndian.AppendUint64(w, u)
			w = binary.LittleEndian.AppendUint32(w, 0)
		}
	}
	return w, nil
}

// lookup returns the arbitration, nil when unknown (expired or lost to a
// restart), or errNotRegistrant when this console never registered for it.
func (s *Store) lookup(k key, machine uint64) (*arb, error) {
	s.expire()
	a := s.m[k]
	if a == nil {
		return nil, nil
	}
	for _, r := range a.regs {
		if r.machine == machine {
			return a, nil
		}
	}
	return nil, errNotRegistrant
}

// extend is TimeExtend: XRL 0xF01, or the UDP form FUN_81919c20 sends 60 s
// before expiry, the header then u16 300 (seconds).
func (h *Handler) extend(machine uint64, body []byte) error {
	_, _, k, rest, err := header(body)
	if err != nil || len(rest) < 2 {
		return errShort
	}
	s := h.Store
	s.mu.Lock()
	defer s.mu.Unlock()
	a, err := s.lookup(k, machine)
	if a != nil {
		if t := s.now().Add(time.Duration(binary.LittleEndian.Uint16(rest))*time.Second + grace); t.After(a.expires) {
			a.expires = t
		}
	}
	return err
}

// report is ReportResult. XRL 0xF02 (FUN_8182d6f0, xon/5): the header with
// flags 0x100 (| 0x4000), then the stats block u32, u32, u16 n, n procs. The
// schema's optional blocks are counted by arguments that are not on the wire;
// XAM's one caller (FUN_81919d40) sets only the stats block. The UDP form
// (xon/16) is the header with flags 1 (| 0x40), then u32 byte length and the
// procs. Procs are statpost procs; they go on to xstats.
func (h *Handler) report(r *http.Request, machine uint64, body []byte) error {
	flags, title, k, rest, err := header(body)
	if err != nil {
		return err
	}
	var procs []byte
	n := 0
	switch {
	case strings.HasPrefix(r.Header.Get("Content-Type"), "xon/16"):
		if flags&1 != 0 {
			if len(rest) < 4 || int(binary.LittleEndian.Uint32(rest)) > len(rest)-4 {
				return errShort
			}
			n, procs = -1, rest[4:4+binary.LittleEndian.Uint32(rest)]
		}
	case flags&0x100 != 0:
		if len(rest) < 10 {
			return errShort
		}
		n, procs = int(binary.LittleEndian.Uint16(rest[8:])), rest[10:]
	}
	list, err := splitProcs(procs, n)
	if err != nil {
		return err
	}
	s := h.Store
	s.mu.Lock()
	a, err := s.lookup(k, machine)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	// Every console of a ranked match reports; the first report of a player's
	// row in a view wins and later copies are dropped, so Sum columns count once.
	// ponytail: first come, not Live's vote across registrants; a console that
	// reports first can write any result. Compare reports here if that matters.
	var keep [][]byte
	if a == nil {
		keep = list
	} else {
		seen := map[[2]uint64]bool{}
		for _, p := range list {
			id, ok := rowKey(p)
			if ok && a.written[id] {
				continue
			}
			if ok {
				seen[id] = true
			}
			keep = append(keep, p)
		}
		for id := range seen {
			a.written[id] = true
		}
	}
	s.mu.Unlock()
	if len(keep) == 0 || h.StatPost == nil {
		return nil
	}
	post := binary.LittleEndian.AppendUint32(nil, title)
	post = binary.LittleEndian.AppendUint16(post, uint16(len(keep)))
	post = append(post, bytes.Join(keep, nil)...)
	return h.StatPost(r.Context(), r.Header, post)
}

// width is the value size of each stats value type (x360-xstats wire.go).
func width(t byte) int {
	switch t {
	case 0, 1:
		return 1
	case 2:
		return 2
	case 3:
		return 4
	case 4, 5, 7:
		return 8
	}
	return 0
}

// splitProcs cuts n procs {u16 proc, u16 m, m x {u8 type, value}} into their
// raw bytes (n < 0: all of b); trailing bytes are left alone.
func splitProcs(b []byte, n int) ([][]byte, error) {
	var out [][]byte
	for i := 0; i != n && (n >= 0 || len(b) > 0); i++ {
		if len(b) < 4 {
			return nil, errShort
		}
		o := 4
		for j := 0; j < int(binary.LittleEndian.Uint16(b[2:])); j++ {
			if o >= len(b) {
				return nil, errShort
			}
			o += 1 + width(b[o])
		}
		if o > len(b) {
			return nil, errShort
		}
		out = append(out, b[:o])
		b = b[o:]
	}
	return out, nil
}

// rowKey is a column proc's (XUID, view): its first two params are u64 (7) and
// int32 (3) (x360-xstats post).
func rowKey(p []byte) ([2]uint64, bool) {
	if len(p) < 4+9+5 || p[4] != 7 || p[13] != 3 {
		return [2]uint64{}, false
	}
	return [2]uint64{binary.LittleEndian.Uint64(p[5:]), uint64(binary.LittleEndian.Uint32(p[14:]))}, true
}

// PostTo returns a StatPost that sends the body to an xstats statpost URL with
// the console's identity headers and the gateway key.
func PostTo(url, gatewayKey string, cl *http.Client) func(context.Context, http.Header, []byte) error {
	return func(ctx context.Context, hdr http.Header, body []byte) error {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "xon/5")
		req.Header.Set(wire.AccountHeader, hdr.Get(wire.AccountHeader))
		req.Header.Set(wire.UsersHeader, hdr.Get(wire.UsersHeader))
		req.Header.Set(wire.GatewayKeyHeader, gatewayKey)
		resp, err := cl.Do(req)
		if err != nil {
			return err
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("xstats statpost: %s", resp.Status)
		}
		return nil
	}
}
