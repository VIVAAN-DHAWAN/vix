package ui

import "strings"

// chatCache caches the expensive full-transcript render of a thread's chat:
// the buildRenderedChat output split into lines plus a visual-row prefix sum.
// Rebuilding is O(transcript) and previously ran on every frame (30 fps while
// the spinner ticks); with the cache it runs only when the transcript, the
// width, or the generation changes. The streaming tail (thinking text,
// assistant buffer, spinner) is intentionally *not* cached — callers append it
// per frame via combineTail, which only measures the tail lines.
type chatCache struct {
	// gen is bumped by invalidate() to catch mutations that don't change
	// len(chatMessages) (e.g. in-place re-render on width change, replay
	// restore, /clear, turn trim).
	gen      int
	builtGen int
	msgCount int
	width    int
	built    bool
	lines    []string
	rowStart []int
	// userInfos are the line positions + flattened text of each user message
	// (turn start) in the rendered transcript, used by the sticky header.
	userInfos []UserMsgInfo
}

// invalidate forces a rebuild on the next cachedChatLines call.
func (c *chatCache) invalidate() { c.gen++ }

// cachedChatLines returns the rendered committed transcript (excluding the
// streaming tail) as lines plus a visual-row prefix sum where rowStart[i] is
// the total visual rows occupied by lines[:i]. The result is rebuilt only when
// the transcript length, width, or cache generation changed. Callers must not
// mutate the returned slices.
func (sess *ThreadState) cachedChatLines(s Styles, innerWidth int) ([]string, []int) {
	c := &sess.chatCache
	if c.built && c.builtGen == c.gen && c.msgCount == len(sess.chatMessages) && c.width == innerWidth {
		return c.lines, c.rowStart
	}
	lines := strings.Split(buildRenderedChat(sess.chatMessages, s, innerWidth), "\n")
	c.built = true
	c.builtGen = c.gen
	c.msgCount = len(sess.chatMessages)
	c.width = innerWidth
	c.lines = lines
	c.rowStart = visualRowPrefix(lines, innerWidth)
	c.userInfos = userMessageInfos(sess.chatMessages, s, innerWidth)
	return c.lines, c.rowStart
}

// cachedUserInfos returns the cached user-message line positions for the
// transcript, rebuilding the cache if necessary. Callers must not mutate the
// returned slice.
func (sess *ThreadState) cachedUserInfos(s Styles, innerWidth int) []UserMsgInfo {
	sess.cachedChatLines(s, innerWidth) // ensure cache is built for this width/gen
	return sess.chatCache.userInfos
}

// visualRowPrefix returns prefix sums of visualRows over lines:
// result[i] is the visual rows occupied by lines[:i], result has len(lines)+1
// entries.
func visualRowPrefix(lines []string, innerWidth int) []int {
	rowStart := make([]int, len(lines)+1)
	for i, line := range lines {
		rowStart[i+1] = rowStart[i] + visualRows(line, innerWidth)
	}
	return rowStart
}

// emptyChatLines reports whether cached transcript lines represent an empty
// rendered chat (buildRenderedChat returned "").
func emptyChatLines(lines []string) bool {
	return len(lines) == 1 && lines[0] == ""
}

// combineTail appends the streaming tail (thinking text, assistant buffer,
// spinner frame) to cached transcript lines, producing exactly what
// strings.Split(content+tail, "\n") plus a full visualRows scan would — but
// measuring only the merged boundary line and the tail. The input slices are
// never mutated. With an empty tail the cached slices are returned as-is.
func combineTail(lines []string, rowStart []int, tail string, innerWidth int) ([]string, []int) {
	if tail == "" {
		return lines, rowStart
	}
	tailLines := strings.Split(tail, "\n")
	n := len(lines)
	out := make([]string, 0, n+len(tailLines)-1)
	out = append(out, lines[:n-1]...)
	out = append(out, lines[n-1]+tailLines[0])
	out = append(out, tailLines[1:]...)

	rs := make([]int, len(out)+1)
	copy(rs, rowStart[:n]) // prefix sums of the untouched lines
	for i := n - 1; i < len(out); i++ {
		rs[i+1] = rs[i] + visualRows(out[i], innerWidth)
	}
	return out, rs
}
