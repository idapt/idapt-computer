package progress

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/mattn/go-isatty"
)

const firstPaintDelay = 400 * time.Millisecond

const renderInterval = 100 * time.Millisecond

func ShouldRender(w io.Writer, isTable bool) bool {
	if !isTable {
		return false
	}
	if f, ok := w.(*os.File); ok {
		return isatty.IsTerminal(f.Fd()) || isatty.IsCygwinTerminal(f.Fd())
	}
	return false
}

type Bar struct {
	w       io.Writer
	label   string
	total   int64
	enabled bool
	start   time.Time
	mu          sync.Mutex
	done        int64
	lastRender  time.Time
	lastLineLen int
}

func NewBar(w io.Writer, label string, total int64, enabled bool) *Bar {
	return &Bar{w: w, label: label, total: total, enabled: enabled, start: time.Now()}
}

func (b *Bar) Add(n int64) {
	if b == nil || !b.enabled {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.done += n
	now := time.Now()
	if now.Sub(b.lastRender) < renderInterval && (b.total <= 0 || b.done < b.total) {
		return
	}
	b.lastRender = now
	b.render(now)
}

func (b *Bar) Done() {
	if b == nil || !b.enabled {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.render(time.Now())
	fmt.Fprintln(b.w)
}

func (b *Bar) render(now time.Time) {
	line := b.formatLine(now.Sub(b.start))
	pad := clearPad(len(line), b.lastLineLen)
	fmt.Fprintf(b.w, "\r%s%s", line, pad)
	b.lastLineLen = len(line)
}

func (b *Bar) formatLine(elapsed time.Duration) string {
	secs := elapsed.Seconds()
	if secs < 0.001 {
		secs = 0.001
	}
	rate := float64(b.done) / secs
	if b.total > 0 {
		pct := int(float64(b.done) / float64(b.total) * 100)
		s := fmt.Sprintf("%s: %s / %s (%d%%) at %s/s", b.label, FormatBytes(float64(b.done)), FormatBytes(float64(b.total)), pct, FormatBytes(rate))
		if rate > 0 && b.done < b.total {
			s += ", ETA " + FormatDuration(float64(b.total-b.done)/rate)
		}
		return s
	}
	return fmt.Sprintf("%s: %s at %s/s", b.label, FormatBytes(float64(b.done)), FormatBytes(rate))
}

func (b *Bar) ProxyReader(r io.Reader) io.Reader { return &proxyReader{r: r, bar: b} }

func (b *Bar) ProxyWriter(w io.Writer) io.Writer { return &proxyWriter{w: w, bar: b} }

type proxyReader struct {
	r   io.Reader
	bar *Bar
}

func (p *proxyReader) Read(buf []byte) (int, error) {
	n, err := p.r.Read(buf)
	if n > 0 {
		p.bar.Add(int64(n))
	}
	return n, err
}

type proxyWriter struct {
	w   io.Writer
	bar *Bar
}

func (p *proxyWriter) Write(buf []byte) (int, error) {
	n, err := p.w.Write(buf)
	if n > 0 {
		p.bar.Add(int64(n))
	}
	return n, err
}

type Spinner struct {
	w           io.Writer
	enabled     bool
	mu          sync.Mutex
	label       string
	start       time.Time
	lastLineLen int
	stop        chan struct{}
	done        chan struct{}
}

func NewSpinner(w io.Writer, label string, enabled bool) *Spinner {
	return &Spinner{w: w, label: label, enabled: enabled}
}

func (s *Spinner) Start() {
	if s == nil || !s.enabled {
		return
	}
	s.start = time.Now()
	s.stop = make(chan struct{})
	s.done = make(chan struct{})
	go s.loop()
}

func (s *Spinner) SetLabel(label string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.label = label
	s.mu.Unlock()
}

func (s *Spinner) Stop() {
	if s == nil || !s.enabled || s.stop == nil {
		return
	}
	close(s.stop)
	<-s.done
	s.mu.Lock()
	if s.lastLineLen > 0 {
		fmt.Fprintf(s.w, "\r%s\r", strings.Repeat(" ", s.lastLineLen))
	}
	s.mu.Unlock()
	s.stop = nil
}

func (s *Spinner) loop() {
	defer close(s.done)
	select {
	case <-time.After(firstPaintDelay):
	case <-s.stop:
		return
	}
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	t := time.NewTicker(renderInterval)
	defer t.Stop()
	for i := 0; ; i++ {
		select {
		case <-s.stop:
			return
		case <-t.C:
			s.render(frames[i%len(frames)])
		}
	}
}

func (s *Spinner) render(frame string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	elapsed := time.Since(s.start).Round(time.Second)
	line := fmt.Sprintf("%s %s (%s)", frame, s.label, elapsed)
	pad := clearPad(len(line), s.lastLineLen)
	fmt.Fprintf(s.w, "\r%s%s", line, pad)
	s.lastLineLen = len(line)
}

func clearPad(newLen, prevLen int) string {
	if prevLen > newLen {
		return strings.Repeat(" ", prevLen-newLen)
	}
	return ""
}

func FormatBytes(value float64) string {
	const unit = 1024
	if value < unit {
		return fmt.Sprintf("%.0f B", value)
	}
	v := value / unit
	for _, suffix := range []string{"KiB", "MiB", "GiB", "TiB"} {
		if v < unit {
			return fmt.Sprintf("%.1f %s", v, suffix)
		}
		v /= unit
	}
	return fmt.Sprintf("%.1f PiB", v)
}

func FormatDuration(seconds float64) string {
	d := time.Duration(seconds * float64(time.Second)).Round(time.Second)
	if d < time.Minute {
		return d.String()
	}
	return d.Truncate(time.Second).String()
}
