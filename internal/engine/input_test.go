package engine

import "testing"

func collect(t *testing.T, data []byte) []Key {
	t.Helper()
	ch := make(chan Key, 32)
	pending := parseInput(data, ch)
	flushPending(pending, ch)
	close(ch)
	var keys []Key
	for k := range ch {
		keys = append(keys, k)
	}
	return keys
}

func TestParseInputArrowKeysCSI(t *testing.T) {
	cases := []struct {
		seq  []byte
		want KeyCode
	}{
		{[]byte("\x1b[A"), KeyUp},
		{[]byte("\x1b[B"), KeyDown},
		{[]byte("\x1b[C"), KeyRight},
		{[]byte("\x1b[D"), KeyLeft},
	}
	for _, tc := range cases {
		got := collect(t, tc.seq)
		if len(got) != 1 || got[0].Code != tc.want {
			t.Errorf("parseInput(%q) = %+v, want one %v", tc.seq, got, tc.want)
		}
	}
}

func TestParseInputArrowKeysSS3(t *testing.T) {
	cases := []struct {
		seq  []byte
		want KeyCode
	}{
		{[]byte("\x1bOA"), KeyUp},
		{[]byte("\x1bOB"), KeyDown},
		{[]byte("\x1bOC"), KeyRight},
		{[]byte("\x1bOD"), KeyLeft},
	}
	for _, tc := range cases {
		got := collect(t, tc.seq)
		if len(got) != 1 || got[0].Code != tc.want {
			t.Errorf("parseInput(%q) = %+v, want one %v", tc.seq, got, tc.want)
		}
	}
}

func TestParseInputArrowKeySplitAcrossReads(t *testing.T) {
	ch := make(chan Key, 8)
	pending := parseInput([]byte{0x1b}, ch)
	if len(pending) != 1 || pending[0] != 0x1b {
		t.Fatalf("first read pending = %v, want [0x1b]", pending)
	}
	if len(ch) != 0 {
		t.Fatalf("first read emitted %d keys, want 0", len(ch))
	}
	combined := append(pending, []byte("[A")...)
	pending = parseInput(combined, ch)
	if pending != nil {
		t.Errorf("after complete sequence pending = %v, want nil", pending)
	}
	close(ch)
	var keys []Key
	for k := range ch {
		keys = append(keys, k)
	}
	if len(keys) != 1 || keys[0].Code != KeyUp {
		t.Errorf("got %+v, want one KeyUp", keys)
	}
}

func TestParseInputPartialCsiPending(t *testing.T) {
	// "\x1b[" alone is not yet a complete CSI; should be held as pending.
	ch := make(chan Key, 4)
	pending := parseInput([]byte("\x1b["), ch)
	if string(pending) != "\x1b[" {
		t.Errorf("pending = %q, want \"\\x1b[\"", pending)
	}
	if len(ch) != 0 {
		t.Errorf("emitted %d keys, want 0", len(ch))
	}
}

func TestFlushPendingEmitsEsc(t *testing.T) {
	ch := make(chan Key, 4)
	flushPending([]byte{0x1b}, ch)
	flushPending([]byte{0x1b, '['}, ch)
	flushPending(nil, ch)
	close(ch)
	var keys []Key
	for k := range ch {
		keys = append(keys, k)
	}
	if len(keys) != 2 || keys[0].Code != KeyEsc || keys[1].Code != KeyEsc {
		t.Errorf("got %+v, want two KeyEsc", keys)
	}
}

func TestParseInputBareEsc(t *testing.T) {
	got := collect(t, []byte{0x1b})
	if len(got) != 1 || got[0].Code != KeyEsc {
		t.Errorf("got %+v, want one KeyEsc", got)
	}
}

func TestParseInputEnterAndChars(t *testing.T) {
	got := collect(t, []byte("a\r b"))
	if len(got) != 4 {
		t.Fatalf("got %d keys, want 4: %+v", len(got), got)
	}
	if got[0].Code != KeyChar || got[0].Rune != 'a' {
		t.Errorf("got[0] = %+v, want KeyChar 'a'", got[0])
	}
	if got[1].Code != KeyEnter {
		t.Errorf("got[1] = %+v, want KeyEnter", got[1])
	}
	if got[2].Code != KeyChar || got[2].Rune != ' ' {
		t.Errorf("got[2] = %+v, want KeyChar ' '", got[2])
	}
	if got[3].Code != KeyChar || got[3].Rune != 'b' {
		t.Errorf("got[3] = %+v, want KeyChar 'b'", got[3])
	}
}

func TestParseInputBackspaceAndTab(t *testing.T) {
	got := collect(t, []byte{0x7f, '\t', '\b'})
	if len(got) != 3 {
		t.Fatalf("got %d keys, want 3", len(got))
	}
	if got[0].Code != KeyBackspace {
		t.Errorf("got[0] = %+v, want KeyBackspace", got[0])
	}
	if got[1].Code != KeyTab {
		t.Errorf("got[1] = %+v, want KeyTab", got[1])
	}
	if got[2].Code != KeyBackspace {
		t.Errorf("got[2] = %+v, want KeyBackspace", got[2])
	}
}

func TestParseInputUnknownCSIIsDropped(t *testing.T) {
	got := collect(t, []byte("\x1b[15~x"))
	if len(got) != 1 || got[0].Code != KeyChar || got[0].Rune != 'x' {
		t.Errorf("got %+v, want only KeyChar 'x'", got)
	}
}

func TestPollKeyEmpty(t *testing.T) {
	e := &Engine{inputCh: make(chan Key, 4)}
	if k, ok := e.PollKey(); ok {
		t.Errorf("PollKey() on empty = %+v, true; want false", k)
	}
}

func TestPollKeyReturnsQueued(t *testing.T) {
	e := &Engine{inputCh: make(chan Key, 4)}
	e.inputCh <- Key{Code: KeyEsc}
	e.inputCh <- Key{Code: KeyChar, Rune: 'a'}
	k1, ok := e.PollKey()
	if !ok || k1.Code != KeyEsc {
		t.Errorf("first PollKey = %+v ok=%v, want KeyEsc", k1, ok)
	}
	k2, ok := e.PollKey()
	if !ok || k2.Code != KeyChar || k2.Rune != 'a' {
		t.Errorf("second PollKey = %+v ok=%v, want KeyChar 'a'", k2, ok)
	}
	if _, ok := e.PollKey(); ok {
		t.Errorf("third PollKey ok=true, want false")
	}
}
