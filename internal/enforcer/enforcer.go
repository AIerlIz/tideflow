package enforcer

import (
	"context"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"tideflow/internal/storage"
)

// ---- types ----

type failureEntry struct {
	failures int
	ts       time.Time
}

// Engine manages background bandwidth consumption.
type Engine struct {
	store *storage.Store
	mu    sync.RWMutex

	// pause flags (private, accessed via methods)
	pausedByCap    bool
	pausedByWindow bool

	// shared state (public for handler reads; use RLock)
	InWindow      bool
	CooldownIDs   []int
	AllFailed     bool
	PausedByCap   bool
	PausedByWindow bool

	// tasks
	taskID       int
	streamCancel map[string]context.CancelFunc
	StreamBytes  map[string]int64
	TaskSource   map[string]int

	// traffic
	TrafficThisPeriod int64
	trafficCap        int64

	// speed
	DownloadSpeed  int64
	lastBytesTotal int64
	lastSampleTime time.Time

	// failure tracker: sourceID -> entry
	failureTracker map[int]failureEntry
}

// NewEngine creates an Engine bound to the given store.
func NewEngine(store *storage.Store) *Engine {
	return &Engine{
		store:          store,
		streamCancel:   make(map[string]context.CancelFunc),
		StreamBytes:    make(map[string]int64),
		TaskSource:     make(map[string]int),
		failureTracker: make(map[int]failureEntry),
		InWindow:       true,
	}
}

// ---- exported accessors ----

// Mu returns the engine's mutex for external synchronized reads.
func (e *Engine) Mu() *sync.RWMutex {
	return &e.mu
}

// GetSetting reads a global setting value (public wrapper).
func (e *Engine) GetSetting(key string) string {
	return e.getSetting(key)
}

// ResetFailures clears the failure tracker and cooldown state.
func (e *Engine) ResetFailures() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.failureTracker = make(map[int]failureEntry)
	e.CooldownIDs = nil
	e.AllFailed = false
}

// FailureCount returns the number of tracked failures.
func (e *Engine) FailureCount() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.failureTracker)
}

// ---- helpers ----

func (e *Engine) getSetting(key string) string {
	return e.store.GetSetting(key)
}

func (e *Engine) currentRecord() *storage.TrafficRecord {
	return e.store.CurrentRecord()
}

func (e *Engine) syncTraffic() {
	e.mu.RLock()
	tp := e.TrafficThisPeriod
	e.mu.RUnlock()
	e.store.SyncTraffic(tp)
}

func (e *Engine) sources() []storage.SourceRecord {
	return e.store.ListEnabledSources()
}

// ---- window / cap checks ----

func inWindow(startStr, endStr string) bool {
	now := time.Now()
	parse := func(s string) (int, int, bool) {
		parts := strings.Split(s, ":")
		if len(parts) != 2 {
			return 0, 0, false
		}
		h, _ := strconv.Atoi(parts[0])
		m, _ := strconv.Atoi(parts[1])
		return h, m, true
	}
	sh, sm, ok1 := parse(startStr)
	eh, em, ok2 := parse(endStr)
	if !ok1 || !ok2 {
		return true
	}
	s := time.Date(now.Year(), now.Month(), now.Day(), sh, sm, 0, 0, now.Location())
	e := time.Date(now.Year(), now.Month(), now.Day(), eh, em, 0, 0, now.Location())
	nt := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(), now.Second(), 0, now.Location())

	if s.Before(e) || s.Equal(e) {
		return (nt.After(s) || nt.Equal(s)) && (nt.Before(e) || nt.Equal(e))
	}
	// crossing midnight
	return (nt.After(s) || nt.Equal(s)) || (nt.Before(e) || nt.Equal(e))
}

func (e *Engine) canDownload() bool {
	if e.getSetting("time_window_enabled") == "true" {
		if !inWindow(e.getSetting("time_window_start"), e.getSetting("time_window_end")) {
			return false
		}
	}
	if e.getSetting("traffic_cap_enabled") == "true" {
		rec := e.currentRecord()
		if rec != nil {
			capStr := e.getSetting("traffic_cap_bytes")
			cap, _ := strconv.ParseInt(capStr, 10, 64)
			e.mu.RLock()
			tp := e.TrafficThisPeriod
			e.mu.RUnlock()
			if cap > 0 && tp >= cap {
				return false
			}
		}
	}
	return true
}

// ---- period management ----

func (e *Engine) initPeriod() {
	if rec := e.currentRecord(); rec != nil {
		return
	}
	pt := e.getSetting("traffic_cap_period")
	rh, _ := strconv.Atoi(e.getSetting("traffic_cap_reset_hour"))
	rd, _ := strconv.Atoi(e.getSetting("traffic_cap_reset_day"))
	if rd < 1 {
		rd = 1
	}
	if rd > 28 {
		rd = 28
	}

	now := time.Now()
	var end time.Time

	switch pt {
	case "daily":
		rst := time.Date(now.Year(), now.Month(), now.Day(), rh, 0, 0, 0, now.Location())
		if now.Before(rst) {
			end = rst
		} else {
			end = rst.Add(24 * time.Hour)
		}
	case "weekly":
		// Reset weekday: 1=Mon … 7=Sun (default Monday)
		rw, _ := strconv.Atoi(e.getSetting("traffic_cap_reset_weekday"))
		if rw < 1 || rw > 7 {
			rw = 1
		}
		// Map 1..7 (Mon..Sun) to Go's time.Weekday (0=Sun, 1..6=Mon..Sat)
		var targetWD time.Weekday
		if rw == 7 {
			targetWD = time.Sunday
		} else {
			targetWD = time.Weekday(rw) // 1=Mon matches time.Monday
		}
		// Find the target day in the current week
		nowWD := now.Weekday()
		daysSinceTarget := int(nowWD - targetWD)
		if daysSinceTarget < 0 {
			daysSinceTarget += 7
		}
		resetDay := now.Add(-time.Duration(daysSinceTarget) * 24 * time.Hour)
		resetDay = time.Date(resetDay.Year(), resetDay.Month(), resetDay.Day(), rh, 0, 0, 0, now.Location())
		if now.Before(resetDay) {
			end = resetDay
		} else {
			end = resetDay.Add(7 * 24 * time.Hour)
		}
	case "monthly":
		thisM := time.Date(now.Year(), now.Month(), rd, rh, 0, 0, 0, now.Location())
		if now.Before(thisM) {
			end = thisM
		} else {
			next := now.AddDate(0, 1, 0)
			end = time.Date(next.Year(), next.Month(), rd, rh, 0, 0, 0, now.Location())
		}
	default:
		end = now.Add(24 * time.Hour)
	}

	e.store.CreateRecord(now, end, pt)
}

func (e *Engine) resetPeriod() {
	e.syncTraffic()

	now := time.Now()
	pt := e.getSetting("traffic_cap_period")
	rh, _ := strconv.Atoi(e.getSetting("traffic_cap_reset_hour"))
	rd, _ := strconv.Atoi(e.getSetting("traffic_cap_reset_day"))
	if rd < 1 {
		rd = 1
	}
	if rd > 28 {
		rd = 28
	}

	var end time.Time
	switch pt {
	case "daily":
		rst := time.Date(now.Year(), now.Month(), now.Day(), rh, 0, 0, 0, now.Location())
		if now.Before(rst) {
			end = rst
		} else {
			end = rst.Add(24 * time.Hour)
		}
	case "weekly":
		rw, _ := strconv.Atoi(e.getSetting("traffic_cap_reset_weekday"))
		if rw < 1 || rw > 7 {
			rw = 1
		}
		var targetWD time.Weekday
		if rw == 7 {
			targetWD = time.Sunday
		} else {
			targetWD = time.Weekday(rw)
		}
		nowWD := now.Weekday()
		daysSinceTarget := int(nowWD - targetWD)
		if daysSinceTarget < 0 {
			daysSinceTarget += 7
		}
		resetDay := now.Add(-time.Duration(daysSinceTarget) * 24 * time.Hour)
		resetDay = time.Date(resetDay.Year(), resetDay.Month(), resetDay.Day(), rh, 0, 0, 0, now.Location())
		if now.Before(resetDay) {
			end = resetDay
		} else {
			end = resetDay.Add(7 * 24 * time.Hour)
		}
	case "monthly":
		thisM := time.Date(now.Year(), now.Month(), rd, rh, 0, 0, 0, now.Location())
		if now.Before(thisM) {
			end = thisM
		} else {
			next := now.AddDate(0, 1, 0)
			end = time.Date(next.Year(), next.Month(), rd, rh, 0, 0, 0, now.Location())
		}
	default:
		end = now.Add(24 * time.Hour)
	}

	e.store.ResetPeriod(now, now, end, pt)

	e.mu.Lock()
	e.TrafficThisPeriod = 0
	e.failureTracker = make(map[int]failureEntry)
	e.CooldownIDs = nil
	e.AllFailed = false
	e.mu.Unlock()
	log.Println("Period reset")
}

// ---- cooldown ----

func cooldownSecs(failures int) time.Duration {
	switch {
	case failures <= 1:
		return 30 * time.Second
	case failures == 2:
		return 300 * time.Second
	default:
		return 1800 * time.Second
	}
}

// ---- stream download ----

func (e *Engine) stream(ctx context.Context, taskKey string, sourceID int, url, name, limit string) {
	total := int64(0)
	defer func() {
		e.mu.Lock()
		delete(e.streamCancel, taskKey)
		delete(e.StreamBytes, taskKey)
		delete(e.TaskSource, taskKey)
		e.mu.Unlock()
		// fill slots asynchronously
		go e.fill()
	}()

	var bps int64
	if limit != "" {
		s := strings.ToUpper(limit)
		switch {
		case strings.HasSuffix(s, "M"):
			v, _ := strconv.ParseFloat(strings.TrimSuffix(s, "M"), 64)
			bps = int64(v * 1048576)
		case strings.HasSuffix(s, "K"):
			v, _ := strconv.ParseFloat(strings.TrimSuffix(s, "K"), 64)
			bps = int64(v * 1024)
		default:
			bps, _ = strconv.ParseInt(s, 10, 64)
		}
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		e.recordFailure(sourceID, fmt.Errorf("create request: %w", err))
		return
	}

	client := &http.Client{
		Timeout: 600 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return nil // follow redirects
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		e.recordFailure(sourceID, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		e.recordFailure(sourceID, fmt.Errorf("HTTP %d", resp.StatusCode))
		return
	}

	t0 := time.Now()
	buf := make([]byte, 1048576) // 1MB chunks
	for {
		select {
		case <-ctx.Done():
			if total > 10240 {
				e.syncTraffic()
				e.mu.Lock()
				delete(e.failureTracker, sourceID)
				e.mu.Unlock()
			}
			return
		default:
		}

		n, err := resp.Body.Read(buf)
		if n > 0 {
			total += int64(n)

			// Read cap BEFORE acquiring lock to avoid reentrant deadlock
			e.mu.RLock()
			cap := e.trafficCap
			e.mu.RUnlock()

			capped := false
			e.mu.Lock()
			e.StreamBytes[taskKey] = total
			e.TrafficThisPeriod += int64(n)

			// check cap overflow (still under lock)
			if cap > 0 && e.TrafficThisPeriod > cap {
				excess := e.TrafficThisPeriod - cap
				e.TrafficThisPeriod = cap
				total -= excess
				e.StreamBytes[taskKey] = total
				e.pausedByCap = true
				e.PausedByCap = true
				capped = true
			}
			e.mu.Unlock()

			if capped {
				goto done
			}

			// speed limiting (outside lock)
			if bps > 0 {
				elapsed := time.Since(t0).Seconds()
				expected := float64(total) / float64(bps)
				if elapsed < expected {
					time.Sleep(time.Duration((expected-elapsed)*1000) * time.Millisecond)
				}
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			e.recordFailure(sourceID, err)
			return
		}
	}

done:
	if total > 10240 {
		e.syncTraffic()
		e.mu.Lock()
		delete(e.failureTracker, sourceID)
		e.mu.Unlock()
		log.Printf("✓ %s: %.1fGB", name, float64(total)/1073741824)
	} else {
		e.recordFailure(sourceID, fmt.Errorf("too small: %d bytes", total))
	}
}

func (e *Engine) recordFailure(sourceID int, err error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	entry := e.failureTracker[sourceID]
	entry.failures++
	entry.ts = time.Now()
	e.failureTracker[sourceID] = entry
	log.Printf("✗ source %d (%dx): %v", sourceID, entry.failures, err)
}

// ---- task control ----

// StopOne cancels a single download task.
func (e *Engine) StopOne(taskKey string) {
	e.mu.Lock()
	cancel, ok := e.streamCancel[taskKey]
	delete(e.streamCancel, taskKey)
	delete(e.StreamBytes, taskKey)
	delete(e.TaskSource, taskKey)
	e.mu.Unlock()
	if ok && cancel != nil {
		cancel()
	}
}

// StopAll cancels every running download task.
func (e *Engine) StopAll() {
	e.mu.RLock()
	keys := make([]string, 0, len(e.streamCancel))
	for k := range e.streamCancel {
		keys = append(keys, k)
	}
	e.mu.RUnlock()
	for _, k := range keys {
		e.StopOne(k)
	}
}

// ---- fill slots ----

func (e *Engine) fill() {
	// Always prune excess tasks first (even when paused)
	maxCC, _ := strconv.Atoi(e.getSetting("max_concurrent"))
	if maxCC < 1 {
		maxCC = 1
	}
	e.mu.RLock()
	activeCount := len(e.streamCancel)
	e.mu.RUnlock()
	if activeCount > maxCC {
		excess := activeCount - maxCC
		e.mu.RLock()
		var keys []string
		for k := range e.streamCancel {
			keys = append(keys, k)
		}
		e.mu.RUnlock()
		for i := 0; i < excess && i < len(keys); i++ {
			e.StopOne(keys[i])
		}
		// Re-read after pruning
		e.mu.RLock()
		activeCount = len(e.streamCancel)
		e.mu.RUnlock()
	}

	e.mu.RLock()
	pbc := e.pausedByCap
	pbw := e.pausedByWindow
	e.mu.RUnlock()

	if pbc || pbw {
		return
	}
	if !e.canDownload() {
		return
	}

	sources := e.sources()
	if len(sources) == 0 {
		return
	}

	now := time.Now()
	e.mu.Lock()
	var available []storage.SourceRecord
	var cooldownIDs []int

	// Build set of currently downloading source IDs
	busySet := make(map[int]bool)
	for _, sid := range e.TaskSource {
		busySet[sid] = true
	}

	for _, s := range sources {
		if ent, ok := e.failureTracker[s.ID]; ok {
			if now.Sub(ent.ts) < cooldownSecs(ent.failures) {
				cooldownIDs = append(cooldownIDs, s.ID)
				continue
			}
			delete(e.failureTracker, s.ID)
		}
		available = append(available, s)
	}
	e.CooldownIDs = cooldownIDs
	e.AllFailed = len(sources) > 0 && len(available) == 0 && len(e.streamCancel) == 0
	e.mu.Unlock()

	if len(available) == 0 {
		return
	}

	need := maxCC - activeCount
	if need <= 0 {
		return
	}
	if need > len(available) {
		need = len(available)
	}

	// Split into idle (not downloading) and busy (already downloading).
	// Prefer idle sources to spread load across all available sources.
	var idle, busy []storage.SourceRecord
	for _, s := range available {
		if busySet[s.ID] {
			busy = append(busy, s)
		} else {
			idle = append(idle, s)
		}
	}

	// Shuffle both groups independently
	rand.Shuffle(len(idle), func(i, j int) { idle[i], idle[j] = idle[j], idle[i] })
	rand.Shuffle(len(busy), func(i, j int) { busy[i], busy[j] = busy[j], busy[i] })

	// Pick idle first, then busy to fill remaining slots
	pick := make([]storage.SourceRecord, 0, need)
	if len(idle) > 0 {
		pick = append(pick, idle...)
		if len(pick) > need {
			pick = pick[:need]
		}
	}
	if len(pick) < need && len(busy) > 0 {
		rem := need - len(pick)
		if rem > len(busy) {
			rem = len(busy)
		}
		pick = append(pick, busy[:rem]...)
	}

	speed := e.getSetting("default_max_speed")
	if speed == "0" {
		speed = ""
	}

	e.mu.Lock()
	for _, s := range pick {
		e.taskID++
		key := fmt.Sprintf("%d-%d", s.ID, e.taskID)
		log.Printf("▶ %s [%s]", s.Name, key)
		ctx, cancel := context.WithCancel(context.Background())
		e.streamCancel[key] = cancel
		e.TaskSource[key] = s.ID
		e.StreamBytes[key] = 0
		go e.stream(ctx, key, s.ID, s.URL, s.Name, speed)
	}
	e.mu.Unlock()
}

// ---- speed calculation ----

func (e *Engine) calcSpeed() {
	e.mu.Lock()
	defer e.mu.Unlock()
	now := time.Now()
	var current int64
	for _, b := range e.StreamBytes {
		current += b
	}
	if !e.lastSampleTime.IsZero() {
		elapsed := now.Sub(e.lastSampleTime).Seconds()
		if elapsed > 0 {
			e.DownloadSpeed = int64(float64(current-e.lastBytesTotal) / elapsed)
		}
	}
	e.lastBytesTotal = current
	e.lastSampleTime = now
}

// ---- main loop ----

// Run starts the enforcer control loop. Blocks until ctx is cancelled.
func (e *Engine) Run(ctx context.Context) {
	log.Println("TideFlow enforcer started")
	e.initPeriod()

	// Restore traffic count from store
	if rec := e.currentRecord(); rec != nil {
		e.mu.Lock()
		e.TrafficThisPeriod = rec.TotalBytes
		e.mu.Unlock()
	}

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			e.StopAll()
			e.syncTraffic()
			log.Println("TideFlow enforcer stopped")
			return
		case <-ticker.C:
			e.calcSpeed()
			e.tick()
		}
	}
}

func (e *Engine) tick() {
	// ---- time window ----
	tw := e.getSetting("time_window_enabled")
	if tw == "true" {
		inW := inWindow(e.getSetting("time_window_start"), e.getSetting("time_window_end"))
		e.mu.Lock()
		e.InWindow = inW
		pbw := e.pausedByWindow
		if !inW && !pbw {
			e.pausedByWindow = true
			e.PausedByWindow = true
			e.mu.Unlock()
			e.StopAll()
			log.Println("Outside window → PAUSED")
		} else if inW && pbw {
			e.pausedByWindow = false
			e.PausedByWindow = false
			e.mu.Unlock()
		} else {
			e.mu.Unlock()
		}
	} else {
		e.mu.Lock()
		if e.pausedByWindow {
			e.pausedByWindow = false
			e.PausedByWindow = false
		}
		e.mu.Unlock()
	}

	// ---- traffic cap ----
	ce := e.getSetting("traffic_cap_enabled")
	if ce == "true" {
		cap, _ := strconv.ParseInt(e.getSetting("traffic_cap_bytes"), 10, 64)
		e.mu.Lock()
		e.trafficCap = cap
		tp := e.TrafficThisPeriod
		pbc := e.pausedByCap
		if cap > 0 && tp >= cap && !pbc {
			e.pausedByCap = true
			e.PausedByCap = true
			e.mu.Unlock()
			e.StopAll()
			log.Printf("Cap %d/%d → PAUSED", tp, cap)
		} else if tp < cap && pbc {
			e.pausedByCap = false
			e.PausedByCap = false
			e.mu.Unlock()
		} else {
			e.mu.Unlock()
		}
	} else {
		e.mu.Lock()
		e.trafficCap = 0
		if e.pausedByCap {
			e.pausedByCap = false
			e.PausedByCap = false
		}
		e.mu.Unlock()
	}

	// ---- fill ----
	// Always call fill() — pruning must work even when paused.
	e.fill()

	// ---- period reset ----
	rec := e.currentRecord()
	if rec != nil && time.Now().After(rec.PeriodEnd) {
		e.StopAll()
		e.resetPeriod()
		e.mu.Lock()
		e.pausedByCap = false
		e.PausedByCap = false
		e.mu.Unlock()
	}
}
