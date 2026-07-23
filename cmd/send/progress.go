package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/mattn/go-isatty"
)

// redrawInterval keeps the bar from flooding a slow terminal — or a CI log,
// where it degrades to a single line printed at the end.
const redrawInterval = 100 * time.Millisecond

// progressBar renders transfer progress on stderr. On a non-TTY it stays
// silent until done, so piping output stays clean.
type progressBar struct {
	label string
	total int64

	mu         sync.Mutex
	current    int64
	started    time.Time
	lastRedraw time.Time
	tty        bool
	finished   bool
}

func newProgressBar(label string, total int64) *progressBar {
	return &progressBar{
		label:   label,
		total:   total,
		started: time.Now(),
		tty:     isatty.IsTerminal(os.Stderr.Fd()) || isatty.IsCygwinTerminal(os.Stderr.Fd()),
	}
}

// wrap returns a reader that advances the bar as it is consumed.
func (b *progressBar) wrap(r io.Reader) io.Reader {
	return &progressReader{reader: r, bar: b}
}

func (b *progressBar) add(n int64) {
	b.mu.Lock()
	b.current += n
	b.mu.Unlock()
	b.draw(false)
}

func (b *progressBar) set(n int64) {
	b.mu.Lock()
	b.current = n
	b.mu.Unlock()
	b.draw(false)
}

func (b *progressBar) done() {
	b.mu.Lock()
	if b.finished {
		b.mu.Unlock()
		return
	}
	b.finished = true
	b.mu.Unlock()

	b.draw(true)
	fmt.Fprintln(os.Stderr)
}

func (b *progressBar) draw(force bool) {
	if !b.tty && !force {
		return
	}

	b.mu.Lock()
	now := time.Now()
	if !force && now.Sub(b.lastRedraw) < redrawInterval {
		b.mu.Unlock()
		return
	}
	b.lastRedraw = now

	current, total, elapsed := b.current, b.total, now.Sub(b.started)
	b.mu.Unlock()

	rate := float64(current) / elapsed.Seconds()

	var line string
	if total > 0 {
		pct := float64(current) / float64(total)
		if pct > 1 {
			pct = 1
		}

		const width = 24
		filled := int(pct * width)
		bar := strings.Repeat("=", filled) + strings.Repeat(" ", width-filled)

		eta := ""
		if rate > 0 && current < total {
			eta = "  eta " + shortDuration(time.Duration(float64(total-current)/rate)*time.Second)
		}

		line = fmt.Sprintf("%-24s [%s] %3.0f%%  %s/s%s",
			truncate(b.label, 24), bar, pct*100, humanBytes(int64(rate)), eta)
	} else {
		line = fmt.Sprintf("%-24s %s  %s/s",
			truncate(b.label, 24), humanBytes(current), humanBytes(int64(rate)))
	}

	if b.tty {
		fmt.Fprintf(os.Stderr, "\r\033[K%s", line)
		return
	}

	fmt.Fprint(os.Stderr, line)
}

type progressReader struct {
	reader io.Reader
	bar    *progressBar
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.reader.Read(b)
	if n > 0 {
		p.bar.add(int64(n))
	}
	return n, err
}

func shortDuration(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
	default:
		return fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60)
	}
}
