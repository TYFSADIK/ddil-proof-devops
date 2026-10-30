// Package outbox is a durable store-and-forward queue for DDIL links
// (denied, degraded, intermittent, limited bandwidth).
//
// Messages are files: written to tmp/, fsynced, then atomically renamed into
// ready/. A crash never leaves a half-written message visible. Delivery is
// at-least-once; receivers de-duplicate on Message.ID.
// Priority lanes drain highest first, so a 2 kbit/s link carries alerts
// before logs. Expired messages are dropped and counted, never silently lost.
package outbox

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	mrand "math/rand/v2"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type Priority int

const (
	Bulk     Priority = 0 // logs, metrics
	Normal   Priority = 1 // telemetry
	Critical Priority = 2 // alerts, commands acks
)

type Message struct {
	ID       string    `json:"id"`
	Topic    string    `json:"topic"`
	Priority Priority  `json:"priority"`
	Created  time.Time `json:"created"`
	Expires  time.Time `json:"expires"`
	Payload  []byte    `json:"payload"`
	Attempts int       `json:"attempts"`
}

type Outbox struct {
	dir     string
	Expired int
}

func Open(dir string) (*Outbox, error) {
	for _, d := range []string{"tmp", "ready"} {
		if err := os.MkdirAll(filepath.Join(dir, d), 0o750); err != nil {
			return nil, err
		}
	}
	return &Outbox{dir: dir}, nil
}

func newID() string { b := make([]byte, 12); rand.Read(b); return hex.EncodeToString(b) }

func (o *Outbox) Put(topic string, p Priority, ttl time.Duration, payload []byte) (Message, error) {
	now := time.Now().UTC()
	m := Message{ID: newID(), Topic: topic, Priority: p, Created: now, Expires: now.Add(ttl), Payload: payload}
	return m, o.write(m)
}

// filename sorts by (priority desc, created asc).
func (o *Outbox) name(m Message) string {
	return filepath.Join(o.dir, "ready", string(rune('9'-int(m.Priority)))+"-"+m.Created.Format("20060102T150405.000000000")+"-"+m.ID+".json")
}

func (o *Outbox) write(m Message) error {
	b, _ := json.Marshal(m)
	tmp := filepath.Join(o.dir, "tmp", m.ID)
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	f.Close()
	return os.Rename(tmp, o.name(m))
}

func (o *Outbox) pending() ([]string, error) {
	files, err := filepath.Glob(filepath.Join(o.dir, "ready", "*.json"))
	sort.Strings(files)
	return files, err
}

func (o *Outbox) Len() int { f, _ := o.pending(); return len(f) }

// ErrLinkDown should be returned by a Sender when the link is unavailable; Drain stops and waits.
var ErrLinkDown = errors.New("link down")

type Sender func(Message) error

// Drain sends pending messages in priority order until the queue is empty,
// the link fails, or budget bytes are spent (for metered/limited links).
func (o *Outbox) Drain(send Sender, budget int) (sent int, err error) {
	files, err := o.pending()
	if err != nil {
		return 0, err
	}
	spent := 0
	for _, path := range files {
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var m Message
		if json.Unmarshal(b, &m) != nil {
			os.Rename(path, path+".corrupt")
			continue
		}
		if time.Now().After(m.Expires) {
			os.Remove(path)
			o.Expired++
			continue
		}
		if budget > 0 && spent+len(m.Payload) > budget {
			return sent, nil
		}
		if err := send(m); err != nil {
			m.Attempts++
			os.Remove(path)
			o.write(m)
			return sent, err
		}
		spent += len(m.Payload)
		os.Remove(path)
		sent++
	}
	return sent, nil
}

// Backoff returns full-jitter exponential backoff (AWS architecture blog), capped.
func Backoff(attempt int, base, max time.Duration) time.Duration {
	ceil := math.Min(float64(max), float64(base)*math.Pow(2, float64(attempt)))
	return time.Duration(mrand.Float64() * ceil)
}
