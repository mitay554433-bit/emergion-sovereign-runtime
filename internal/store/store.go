package store

import (
	"bufio"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"emergion-sovereign-runtime/internal/core"
	"emergion-sovereign-runtime/internal/cosl"
	"emergion-sovereign-runtime/internal/pivot"
)

type Store struct{ Root string }
type Evidence struct {
	Hash          string
	Bytes, Stored int64
	Codec         string
}

func Open(root string) (*Store, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("state root required")
	}
	for _, p := range []string{root, filepath.Join(root, "o")} {
		if err := os.MkdirAll(p, 0o700); err != nil {
			return nil, err
		}
	}
	return &Store{Root: root}, nil
}
func (s *Store) logPath() string               { return filepath.Join(s.Root, "field.cosl") }
func (s *Store) objectPath(hash string) string { return filepath.Join(s.Root, "o", hash+".gz") }

func Hash(content []byte) string { v := sha256.Sum256(content); return hex.EncodeToString(v[:]) }

func (s *Store) Preserve(content []byte) (Evidence, error) {
	if len(content) == 0 {
		return Evidence{}, fmt.Errorf("empty evidence")
	}
	h := Hash(content)
	final := s.objectPath(h)
	if st, err := os.Stat(final); err == nil {
		return Evidence{Hash: h, Bytes: int64(len(content)), Stored: st.Size(), Codec: "gzip"}, nil
	}
	tmp, err := os.CreateTemp(filepath.Join(s.Root, "o"), ".tmp-*")
	if err != nil {
		return Evidence{}, err
	}
	name := tmp.Name()
	ok := false
	defer func() {
		tmp.Close()
		if !ok {
			os.Remove(name)
		}
	}()
	gz := gzip.NewWriter(tmp)
	if _, err = gz.Write(content); err != nil {
		return Evidence{}, err
	}
	if err = gz.Close(); err != nil {
		return Evidence{}, err
	}
	if err = tmp.Sync(); err != nil {
		return Evidence{}, err
	}
	if err = tmp.Close(); err != nil {
		return Evidence{}, err
	}
	if err = os.Rename(name, final); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return Evidence{}, err
		}
	}
	ok = true
	st, err := os.Stat(final)
	if err != nil {
		return Evidence{}, err
	}
	return Evidence{Hash: h, Bytes: int64(len(content)), Stored: st.Size(), Codec: "gzip"}, nil
}

func (s *Store) InspectEvidence(hash string) (Evidence, error) {
	b, err := s.ReadEvidence(hash)
	if err != nil {
		return Evidence{}, err
	}

	st, err := os.Stat(s.objectPath(hash))
	if err != nil {
		return Evidence{}, err
	}

	return Evidence{
		Hash:   hash,
		Bytes:  int64(len(b)),
		Stored: st.Size(),
		Codec:  "gzip",
	}, nil
}

func (s *Store) ReadEvidence(hash string) ([]byte, error) {
	f, err := os.Open(s.objectPath(hash))
	if err != nil {
		return nil, fmt.Errorf("missing evidence hash=%s: %w", hash, err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	b, err := io.ReadAll(gz)
	if err != nil {
		return nil, err
	}
	if Hash(b) != hash {
		return nil, fmt.Errorf("evidence hash mismatch hash=%s", hash)
	}
	return b, nil
}

func (s *Store) Events() ([]core.Event, error) {
	f, err := os.Open(s.logPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	var out []core.Event
	prev := ""
	line := 0
	for sc.Scan() {
		line++
		e, err := cosl.Decode(sc.Text())
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		if e.PrevHash != prev {
			return nil, fmt.Errorf(
				"line %d: chain mismatch%s prev_claimed=%s prev_expected=%s",
				line,
				eventIdentityLabel(e),
				e.PrevHash,
				prev,
			)
		}
		prev = e.SelfHash
		out = append(out, e)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) FindBySourceHash(hash string) (core.EmergION, bool, error) {
	ev, err := s.Events()
	if err != nil {
		return core.EmergION{}, false, err
	}
	for _, e := range ev {
		if e.EmergION != nil && e.EmergION.MEM.SourceHash == hash {
			return *e.EmergION, true, nil
		}
	}
	return core.EmergION{}, false, nil
}

func (s *Store) append(kind, subject string, em *core.EmergION, d *core.DecisionReceipt, r *core.REGReceipt, q *core.ActionAuthorizationReceipt) (string, error) {
	unlock, err := s.lock()
	if err != nil {
		return "", err
	}
	defer unlock()
	return s.appendLocked(kind, subject, em, d, r, q)
}

func (s *Store) appendLocked(kind, subject string, em *core.EmergION, d *core.DecisionReceipt, r *core.REGReceipt, q *core.ActionAuthorizationReceipt) (string, error) {
	events, err := s.Events()
	if err != nil {
		return "", err
	}
	if kind == "C" && em != nil {
		for _, event := range events {
			if event.EmergION != nil && event.EmergION.MEM.SourceHash == em.MEM.SourceHash {
				return event.EmergION.IDN, nil
			}
		}
	}
	prev := ""
	if len(events) > 0 {
		prev = events[len(events)-1].SelfHash
	}
	id, _, err := s.writeAfter(prev, kind, subject, em, d, r, q)
	return id, err
}

// writeAfter seals one event chained to prev and appends it as one fsynced
// COSL line. It returns the event ID and the sealed self hash so a caller
// holding the lock can chain the next event without re-reading the ledger.
func (s *Store) writeAfter(prev, kind, subject string, em *core.EmergION, d *core.DecisionReceipt, r *core.REGReceipt, q *core.ActionAuthorizationReceipt) (string, string, error) {
	now := time.Now().UTC()
	seed := []byte(fmt.Sprintf("%d:%s|%d:%s|%d|%d:%s", len(kind), kind, len(subject), subject, now.UnixNano(), len(prev), prev))
	h := sha256.Sum256(seed)
	e := core.Event{Type: kind, ID: "EV-" + strings.ToUpper(hex.EncodeToString(h[:8])), At: now, EmergION: em, Decision: d, REG: r, ActionAuthorization: q, PrevHash: prev}
	line, err := cosl.Encode(e)
	if err != nil {
		return "", "", err
	}
	selfHash := ""
	_, err = pivot.Observe(
		"COSL_APPEND",
		"ENCODE",
		"DECODE",
		"ONE_EVENT_ONE_PHYSICAL_LINE_AND_IDENTITY_STABLE",
		func() error {
			if strings.ContainsAny(line, "\r\n") {
				return fmt.Errorf(
					"event %s encoded across multiple physical lines",
					e.ID,
				)
			}

			decoded, decodeErr := cosl.Decode(line)
			if decodeErr != nil {
				return fmt.Errorf(
					"event %s failed reciprocal decode: %w",
					e.ID,
					decodeErr,
				)
			}
			if decoded.ID != e.ID || decoded.Type != e.Type {
				return fmt.Errorf(
					"event %s changed identity during reciprocal decode",
					e.ID,
				)
			}
			selfHash = decoded.SelfHash
			return nil
		},
	)
	if err != nil {
		return "", "", err
	}
	f, err := os.OpenFile(s.logPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return "", "", err
	}
	if _, err = f.WriteString(line + "\n"); err != nil {
		f.Close()
		return "", "", err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return "", "", err
	}
	if err = f.Close(); err != nil {
		return "", "", err
	}
	return e.ID, selfHash, nil
}

func (s *Store) SaveCandidate(em core.EmergION) (string, error) {
	if em.STA != core.StateAtGOV || !em.VAL.Recoil || !em.VAL.WVC || em.EVO.Version < 1 {
		return "", fmt.Errorf("candidate is not GOV-ready")
	}
	if err := em.EVO.Metadata.Validate(); err != nil {
		return "", fmt.Errorf("candidate metadata invalid: %w", err)
	}
	if err := s.requirePreservedSource(em.MEM.SourceHash); err != nil {
		return "", err
	}
	return s.append("C", em.IDN, &em, nil, nil, nil)
}

// requirePreservedSource is the GOV-admission check that a source hash
// corresponds to a preserved evidence object. coverage() is unchanged.
func (s *Store) requirePreservedSource(hash string) error {
	hash = strings.TrimSpace(hash)
	if hash == "" {
		return fmt.Errorf("candidate source hash has no preserved evidence hash=%q", hash)
	}
	if _, err := s.ReadEvidence(hash); err != nil {
		return fmt.Errorf("candidate source hash has no preserved evidence hash=%s: %w", hash, err)
	}
	return nil
}

// AppendInterpretationRevisionAtTip atomically appends an interpretation revision
// only while the canonical ledger remains at expectedTip. Lifecycle eligibility
// is proven by the caller from canonical replay before this storage operation.

func (s *Store) AppendInterpretationRevisionAtTip(em core.EmergION, expectedTip string) (string, error) {
	if em.STA != core.StateAtGOV || !em.VAL.Recoil || !em.VAL.WVC || em.EVO.Version < 1 {
		return "", fmt.Errorf("interpretation revision is not GOV-ready")
	}
	if em.EVO.Metadata == nil {
		return "", fmt.Errorf("interpretation revision metadata missing")
	}
	if err := em.EVO.Metadata.Validate(); err != nil {
		return "", fmt.Errorf("interpretation revision metadata invalid: %w", err)
	}

	expectedTip = strings.TrimSpace(expectedTip)
	if expectedTip == "" {
		return "", fmt.Errorf("interpretation revision state tip required")
	}

	unlock, err := s.lock()
	if err != nil {
		return "", err
	}
	defer unlock()

	events, err := s.Events()
	if err != nil {
		return "", err
	}
	currentTip := ""
	if len(events) > 0 {
		currentTip = events[len(events)-1].SelfHash
	}
	if currentTip != expectedTip {
		return "", fmt.Errorf(
			"interpretation revision state changed: expected tip %s got %s",
			expectedTip,
			currentTip,
		)
	}

	existing, ok, err := s.FindBySourceHash(em.MEM.SourceHash)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf(
			"interpretation revision source not found: %s",
			em.MEM.SourceHash,
		)
	}

	if em.IDN != existing.IDN ||
		em.MEM.SourceHash != existing.MEM.SourceHash ||
		em.MEM.Bytes != existing.MEM.Bytes ||
		em.MEM.Stored != existing.MEM.Stored ||
		em.MEM.Codec != existing.MEM.Codec ||
		em.MEM.Provenance != existing.MEM.Provenance {
		return "", fmt.Errorf("interpretation revision changed evidence identity")
	}

	if err := s.requirePreservedSource(em.MEM.SourceHash); err != nil {
		return "", err
	}

	return s.appendLocked("I", em.IDN, &em, nil, nil, nil)
}

func (s *Store) SaveDecision(r core.DecisionReceipt) (string, error) {
	return s.append("D", r.EmergIONID, nil, &r, nil, nil)
}
func (s *Store) SaveAccepted(r core.REGReceipt) (string, error) {
	return s.append("R", r.EmergIONID, nil, nil, &r, nil)
}

// BatchWriter appends governance receipts inside one held state lock,
// chaining each event to the previous one in memory. Lines, event IDs and
// hashes are produced exactly as SaveDecision/SaveAccepted produce them.
type BatchWriter struct {
	s    *Store
	prev string
}

func (b *BatchWriter) SaveDecision(r core.DecisionReceipt) (string, error) {
	id, self, err := b.s.writeAfter(b.prev, "D", r.EmergIONID, nil, &r, nil, nil)
	if err == nil {
		b.prev = self
	}
	return id, err
}

func (b *BatchWriter) SaveAccepted(r core.REGReceipt) (string, error) {
	id, self, err := b.s.writeAfter(b.prev, "R", r.EmergIONID, nil, nil, &r, nil)
	if err == nil {
		b.prev = self
	}
	return id, err
}

// Batch holds the state lock once, reads the ledger once, and lets fn append
// receipts through w. An error from fn stops the batch; every line already
// written stays (each is a complete, chained, fsynced event).
func (s *Store) Batch(fn func(events []core.Event, w *BatchWriter) error) error {
	unlock, err := s.lock()
	if err != nil {
		return err
	}
	defer unlock()
	events, err := s.Events()
	if err != nil {
		return err
	}
	w := &BatchWriter{s: s}
	if len(events) > 0 {
		w.prev = events[len(events)-1].SelfHash
	}
	return fn(events, w)
}

func (s *Store) SaveActionAuthorization(r core.ActionAuthorizationReceipt) (string, error) {
	return s.append("Q", r.EmergIONID, nil, nil, nil, &r)
}

func (s *Store) lock() (func(), error) {
	path := filepath.Join(s.Root, ".lock")
	for i := 0; i < 400; i++ {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			f.WriteString(fmt.Sprint(os.Getpid()))
			f.Close()
			return func() { os.Remove(path) }, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		// A lock left by a process that no longer exists (for example a
		// fieldd killed mid-write) is stale: remove it and retry. A lock held
		// by a live process, including this one, is always respected.
		if staleLock(path) {
			os.Remove(path)
			continue
		}
		time.Sleep(25 * time.Millisecond)
	}
	return nil, fmt.Errorf("state is locked")
}

func (s *Store) PruneOrphans() (int, error) {
	events, err := s.Events()
	if err != nil {
		return 0, err
	}
	used := map[string]bool{}
	for _, e := range events {
		if e.EmergION != nil {
			used[e.EmergION.MEM.SourceHash] = true
		}
	}
	entries, err := os.ReadDir(filepath.Join(s.Root, "o"))
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, ent := range entries {
		if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".gz") {
			continue
		}
		h := strings.TrimSuffix(ent.Name(), ".gz")
		if !used[h] {
			if err := os.Remove(filepath.Join(s.Root, "o", ent.Name())); err != nil {
				return removed, err
			}
			removed++
		}
	}
	return removed, nil
}

func (s *Store) VerifyEvidence() (int, error) {
	events, err := s.Events()
	if err != nil {
		return 0, err
	}
	seen := map[string]bool{}
	count := 0
	for i, e := range events {
		if e.EmergION == nil || seen[e.EmergION.MEM.SourceHash] {
			continue
		}
		if _, err := s.ReadEvidence(e.EmergION.MEM.SourceHash); err != nil {
			return count, fmt.Errorf(
				"line %d: evidence%s: %w",
				i+1,
				eventIdentityLabel(e),
				err,
			)
		}
		seen[e.EmergION.MEM.SourceHash] = true
		count++
	}
	return count, nil
}

func eventRecordIDN(e core.Event) string {
	if e.EmergION != nil && e.EmergION.IDN != "" {
		return e.EmergION.IDN
	}
	if e.REG != nil && e.REG.EmergIONID != "" {
		return e.REG.EmergIONID
	}
	if e.Decision != nil && e.Decision.EmergIONID != "" {
		return e.Decision.EmergIONID
	}
	if e.ActionAuthorization != nil && e.ActionAuthorization.EmergIONID != "" {
		return e.ActionAuthorization.EmergIONID
	}
	return ""
}

// eventIdentityLabel names the record identities derivable from a decoded
// event line. It does not change hashing, wire format, or what verify
// accepts or rejects.
func eventIdentityLabel(e core.Event) string {
	var parts []string
	if idn := eventRecordIDN(e); idn != "" {
		parts = append(parts, "IDN="+idn)
	}
	if e.ID != "" {
		parts = append(parts, "event="+e.ID)
	}
	if e.SelfHash != "" {
		parts = append(parts, "hash="+e.SelfHash)
	}
	if e.EmergION != nil && e.EmergION.MEM.SourceHash != "" {
		parts = append(parts, "evidence="+e.EmergION.MEM.SourceHash)
	}
	if len(parts) == 0 {
		return ""
	}
	return " " + strings.Join(parts, " ")
}

// staleLock reports whether the lock file names a pid that is not running.
// An unreadable, empty or own-pid lock is never treated as stale.
func staleLock(path string) bool {
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || pid <= 0 || pid == os.Getpid() {
		return false
	}
	return !pidAlive(pid)
}
