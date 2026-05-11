package engine

import (
	"testing"
	"time"
)

func collectAll(t *testing.T, data []byte) []Key {
	t.Helper()
	var keys []Key
	emit := func(k Key, eventType int) {
		if eventType == kittyEventPress || eventType == kittyEventRepeat {
			keys = append(keys, k)
		}
	}
	pending := parseInput(data, emit, nil, nil)
	flushPending(pending, emit)
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
		got := collectAll(t, tc.seq)
		if len(got) != 1 || got[0].Code != tc.want {
			t.Errorf("parseInput(%q) = %+v, want one %v", tc.seq, got, tc.want)
		}
	}
}

func TestParseInputArrowReleaseHybridLegacy(t *testing.T) {
	// Alacritty (and similar) keeps arrow presses in the plain legacy
	// form but tacks a Kitty-style ;mods:event field onto releases:
	//   press:   "\x1b[A"
	//   release: "\x1b[1;1:3A"
	// The parser must treat the release as event-type 3, not as an
	// unknown CSI to be dropped.
	cases := []struct {
		seq  []byte
		code KeyCode
	}{
		{[]byte("\x1b[1;1:3A"), KeyUp},
		{[]byte("\x1b[1;1:3B"), KeyDown},
		{[]byte("\x1b[1;1:3C"), KeyRight},
		{[]byte("\x1b[1;1:3D"), KeyLeft},
	}
	for _, tc := range cases {
		type ev struct {
			k Key
			e int
		}
		var got []ev
		emit := func(k Key, e int) { got = append(got, ev{k, e}) }
		parseInput(tc.seq, emit, nil, nil)
		if len(got) != 1 {
			t.Errorf("parseInput(%q): got %d events, want 1: %+v", tc.seq, len(got), got)
			continue
		}
		if got[0].k.Code != tc.code {
			t.Errorf("parseInput(%q): got code %v, want %v", tc.seq, got[0].k.Code, tc.code)
		}
		if got[0].e != kittyEventRelease {
			t.Errorf("parseInput(%q): got event %d, want %d (release)", tc.seq, got[0].e, kittyEventRelease)
		}
	}
}

func TestParseInputArrowModifiedPressLegacy(t *testing.T) {
	// "\x1b[1;2A" is Shift+Up legacy form: parameters present but no
	// event-type — should still register as a plain press.
	got := collectAll(t, []byte("\x1b[1;2A"))
	if len(got) != 1 || got[0].Code != KeyUp {
		t.Errorf("got %+v, want one KeyUp press", got)
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
		got := collectAll(t, tc.seq)
		if len(got) != 1 || got[0].Code != tc.want {
			t.Errorf("parseInput(%q) = %+v, want one %v", tc.seq, got, tc.want)
		}
	}
}

func TestParseInputArrowKeySplitAcrossReads(t *testing.T) {
	var keys []Key
	emit := func(k Key, eventType int) {
		if eventType == kittyEventPress {
			keys = append(keys, k)
		}
	}
	pending := parseInput([]byte{0x1b}, emit, nil, nil)
	if len(pending) != 1 || pending[0] != 0x1b {
		t.Fatalf("first read pending = %v, want [0x1b]", pending)
	}
	if len(keys) != 0 {
		t.Fatalf("first read emitted %d keys, want 0", len(keys))
	}
	combined := append(pending, []byte("[A")...)
	pending = parseInput(combined, emit, nil, nil)
	if pending != nil {
		t.Errorf("after complete sequence pending = %v, want nil", pending)
	}
	if len(keys) != 1 || keys[0].Code != KeyUp {
		t.Errorf("got %+v, want one KeyUp", keys)
	}
}

func TestParseInputBareEsc(t *testing.T) {
	got := collectAll(t, []byte{0x1b})
	if len(got) != 1 || got[0].Code != KeyEsc {
		t.Errorf("got %+v, want one KeyEsc", got)
	}
}

func TestParseInputEnterAndChars(t *testing.T) {
	got := collectAll(t, []byte("a\r b"))
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
	got := collectAll(t, []byte{0x7f, '\t', '\b'})
	if len(got) != 3 {
		t.Fatalf("got %d keys, want 3", len(got))
	}
	if got[0].Code != KeyBackspace || got[1].Code != KeyTab || got[2].Code != KeyBackspace {
		t.Errorf("got %+v, want KeyBackspace, KeyTab, KeyBackspace", got)
	}
}

func TestFlushPendingEmitsEsc(t *testing.T) {
	var keys []Key
	emit := func(k Key, eventType int) {
		if eventType == kittyEventPress {
			keys = append(keys, k)
		}
	}
	flushPending([]byte{0x1b}, emit)
	flushPending([]byte{0x1b, '['}, emit)
	flushPending(nil, emit)
	if len(keys) != 2 || keys[0].Code != KeyEsc || keys[1].Code != KeyEsc {
		t.Errorf("got %+v, want two KeyEsc", keys)
	}
}

// --- Kitty CSI u parser -----------------------------------------------------

func TestParseInputKittyArrowKeys(t *testing.T) {
	cases := []struct {
		seq  []byte
		want KeyCode
	}{
		{[]byte("\x1b[57352u"), KeyUp},
		{[]byte("\x1b[57353u"), KeyDown},
		{[]byte("\x1b[57351u"), KeyRight},
		{[]byte("\x1b[57350u"), KeyLeft},
	}
	for _, tc := range cases {
		got := collectAll(t, tc.seq)
		if len(got) != 1 || got[0].Code != tc.want {
			t.Errorf("parseInput(%q) = %+v, want one %v", tc.seq, got, tc.want)
		}
	}
}

func TestParseInputKittyPlainChar(t *testing.T) {
	got := collectAll(t, []byte("\x1b[113u"))
	if len(got) != 1 || got[0].Code != KeyChar || got[0].Rune != 'q' {
		t.Errorf("got %+v, want KeyChar 'q'", got)
	}
}

func TestParseInputKittyShiftedLetterCanonical(t *testing.T) {
	// Codepoint 113 ('q') with shift modifier (mods=2) should canonicalize
	// to upper-case 'Q' so PollKey consumers see the same rune whether the
	// terminal uses Kitty mode or legacy mode.
	got := collectAll(t, []byte("\x1b[113;2u"))
	if len(got) != 1 || got[0].Code != KeyChar || got[0].Rune != 'Q' {
		t.Errorf("got %+v, want KeyChar 'Q'", got)
	}
}

func TestParseInputKittyReleaseNotEmittedAsPress(t *testing.T) {
	// Release events (event-type 3) should never reach PollKey — they're
	// state-only.
	got := collectAll(t, []byte("\x1b[113;1:3u"))
	if len(got) != 0 {
		t.Errorf("release event emitted as press: %+v", got)
	}
}

func TestParseInputKittyRepeatEmitsAsPress(t *testing.T) {
	// Repeat events (event-type 2) should still surface in PollKey so
	// menus keep auto-scrolling when arrows are held.
	got := collectAll(t, []byte("\x1b[57352;1:2u"))
	if len(got) != 1 || got[0].Code != KeyUp {
		t.Errorf("got %+v, want KeyUp on repeat", got)
	}
}

func TestParseInputKittyFlagsReply(t *testing.T) {
	// CSI ? <flags> u — terminal's response to our \x1b[?u query.
	var keys []Key
	gotFlags := -2 // sentinel: callback not called yet
	emit := func(k Key, _ int) { keys = append(keys, k) }
	onFlags := func(f int) { gotFlags = f }

	parseInput([]byte("\x1b[?11u"), emit, onFlags, nil)

	if gotFlags != 11 {
		t.Errorf("onFlags called with %d, want 11", gotFlags)
	}
	if len(keys) != 0 {
		t.Errorf("flags reply leaked as key events: %+v", keys)
	}
}

func TestParseInputKittyFlagsReplyZero(t *testing.T) {
	gotFlags := -2
	onFlags := func(f int) { gotFlags = f }

	parseInput([]byte("\x1b[?0u"), func(Key, int) {}, onFlags, nil)

	if gotFlags != 0 {
		t.Errorf("onFlags = %d, want 0", gotFlags)
	}
}

func TestParseInputKittyFlagsCallbacksOptional(t *testing.T) {
	// Passing nil for either reply callback must not panic.
	parseInput([]byte("\x1b[?11u\x1b[?62;c"), func(Key, int) {}, nil, nil)
}

func TestParseInputDA1Reply(t *testing.T) {
	// CSI ? <list> c — Primary Device Attributes reply.
	var keys []Key
	calls := 0
	emit := func(k Key, _ int) { keys = append(keys, k) }
	onDA := func() { calls++ }

	parseInput([]byte("\x1b[?62;1;6c"), emit, nil, onDA)

	if calls != 1 {
		t.Errorf("onDA called %d times, want 1", calls)
	}
	if len(keys) != 0 {
		t.Errorf("DA reply leaked as key events: %+v", keys)
	}
}

func TestParseInputDA1ShortReply(t *testing.T) {
	// Even the minimal `\x1b[?6c` form must trigger the callback.
	calls := 0
	onDA := func() { calls++ }
	parseInput([]byte("\x1b[?6c"), func(Key, int) {}, nil, onDA)
	if calls != 1 {
		t.Errorf("onDA called %d times, want 1", calls)
	}
}

func TestParseInputKittySpecialKeysByCodepoint(t *testing.T) {
	cases := []struct {
		seq  []byte
		want KeyCode
	}{
		{[]byte("\x1b[27u"), KeyEsc},
		{[]byte("\x1b[13u"), KeyEnter},
		{[]byte("\x1b[9u"), KeyTab},
		{[]byte("\x1b[127u"), KeyBackspace},
	}
	for _, tc := range cases {
		got := collectAll(t, tc.seq)
		if len(got) != 1 || got[0].Code != tc.want {
			t.Errorf("parseInput(%q) = %+v, want one %v", tc.seq, got, tc.want)
		}
	}
}

// --- IsKeyDown / IsCharDown -------------------------------------------------

func TestIsKeyDownTracksPressAndRelease(t *testing.T) {
	e := &Engine{}
	if e.IsKeyDown(KeyUp) {
		t.Errorf("freshly-constructed engine reports KeyUp down")
	}

	e.recordKey(Key{Code: KeyUp}, kittyEventPress)
	if !e.IsKeyDown(KeyUp) {
		t.Errorf("after press KeyUp should be down")
	}

	e.recordKey(Key{Code: KeyUp}, kittyEventRelease)
	if e.IsKeyDown(KeyUp) {
		t.Errorf("after release KeyUp should be up")
	}
}

func TestIsKeyDownConcurrentKeys(t *testing.T) {
	e := &Engine{}
	e.recordKey(Key{Code: KeyUp}, kittyEventPress)
	e.recordKey(Key{Code: KeyRight}, kittyEventPress)
	if !e.IsKeyDown(KeyUp) {
		t.Errorf("KeyUp should be down")
	}
	if !e.IsKeyDown(KeyRight) {
		t.Errorf("KeyRight should be down")
	}
	e.recordKey(Key{Code: KeyUp}, kittyEventRelease)
	if e.IsKeyDown(KeyUp) {
		t.Errorf("KeyUp should be up after release")
	}
	if !e.IsKeyDown(KeyRight) {
		t.Errorf("KeyRight should still be down")
	}
}

func TestIsCharDownTracksChars(t *testing.T) {
	e := &Engine{}
	e.recordKey(Key{Code: KeyChar, Rune: 'w'}, kittyEventPress)
	if !e.IsCharDown('w') {
		t.Errorf("after press 'w' should be down")
	}
	if e.IsCharDown('a') {
		t.Errorf("untouched 'a' should not be down")
	}
	e.recordKey(Key{Code: KeyChar, Rune: 'w'}, kittyEventRelease)
	if e.IsCharDown('w') {
		t.Errorf("after release 'w' should be up")
	}
}

func TestIsKeyDownKittyModeIgnoresDecay(t *testing.T) {
	// In Kitty mode the OS only auto-repeats the most-recently pressed
	// key, so a co-held first key gets no events for the duration of the
	// hold. We must trust release events and skip the decay timer, or
	// diagonals fall apart after a few hundred ms.
	e := &Engine{}
	e.kittyFlags.Store(11) // flag 2 is set → kittyEventsActive() == true

	e.recordKey(Key{Code: KeyUp}, kittyEventPress)
	if !e.IsKeyDown(KeyUp) {
		t.Fatal("KeyUp should be down right after press")
	}

	// Backdate lastSeen well past keyHoldDecay; without our Kitty-mode
	// bypass this would falsely return false.
	e.pressedMu.Lock()
	e.pressedKeys[KeyUp].lastSeen = time.Now().Add(-time.Hour)
	e.pressedMu.Unlock()

	if !e.IsKeyDown(KeyUp) {
		t.Errorf("KeyUp should still be down in Kitty mode regardless of decay")
	}

	// An explicit release event must still take effect.
	e.recordKey(Key{Code: KeyUp}, kittyEventRelease)
	if e.IsKeyDown(KeyUp) {
		t.Errorf("KeyUp should be up after release event")
	}
}

func TestIsKeyDownKittyDetectedViaSeenRelease(t *testing.T) {
	// Even if KittyKeyboardFlags() hasn't reported yet, observing a
	// release in practice should bypass decay from then on.
	e := &Engine{}
	e.recordKey(Key{Code: KeyUp}, kittyEventPress)
	e.recordKey(Key{Code: KeyRight}, kittyEventPress)
	// A release for any key proves the terminal sends them.
	e.recordKey(Key{Code: KeyRight}, kittyEventRelease)

	// Now backdate Up; should still register as down.
	e.pressedMu.Lock()
	e.pressedKeys[KeyUp].lastSeen = time.Now().Add(-time.Hour)
	e.pressedMu.Unlock()

	if !e.IsKeyDown(KeyUp) {
		t.Errorf("KeyUp should remain down once we've seen a release event")
	}
}

func TestIsKeyDownLegacyDecay(t *testing.T) {
	// Legacy terminals never deliver release events. After a press, the
	// key should stay "down" until keyHoldDecay elapses with no further
	// press/repeat. We can't wait 600 ms in a unit test, so we backdate
	// lastSeen and verify the boundary directly.
	e := &Engine{}
	e.recordKey(Key{Code: KeyUp}, kittyEventPress)
	if !e.IsKeyDown(KeyUp) {
		t.Fatalf("KeyUp should be down right after press")
	}
	e.pressedMu.Lock()
	e.pressedKeys[KeyUp].lastSeen = time.Now().Add(-keyHoldDecay - 50*time.Millisecond)
	e.pressedMu.Unlock()
	if e.IsKeyDown(KeyUp) {
		t.Errorf("KeyUp should have decayed to up after %v", keyHoldDecay)
	}
}

func TestIsKeyDownRejectsKeyChar(t *testing.T) {
	// IsKeyDown takes only non-character codes; KeyChar always returns
	// false (use IsCharDown instead).
	e := &Engine{}
	e.recordKey(Key{Code: KeyChar, Rune: 'a'}, kittyEventPress)
	if e.IsKeyDown(KeyChar) {
		t.Errorf("IsKeyDown(KeyChar) should always be false")
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
