package daemon

import (
	"context"
	"log"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

// StartWatch starts a single config watcher loop. Repeated calls are ignored.
func (s *State) StartWatch(ctx context.Context) error {
	s.watchOnce.Do(func() {
		go s.watchLoop(ctx)
	})
	return nil
}

func (s *State) watchLoop(ctx context.Context) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("warn: config watcher disabled: %v", err)
		return
	}
	defer watcher.Close()

	s.addWatchedFiles(watcher)

	debounce := time.NewTimer(time.Hour)
	debounce.Stop()
	pending := false

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if !shouldReloadEvent(event.Name, event.Op) {
				continue
			}
			if !pending {
				pending = true
				debounce.Reset(250 * time.Millisecond)
				continue
			}
			if !debounce.Stop() {
				select {
				case <-debounce.C:
				default:
				}
			}
			debounce.Reset(250 * time.Millisecond)
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Printf("warn: config watcher error: %v", err)
		case <-debounce.C:
			if !pending {
				continue
			}
			pending = false
			if _, err := s.Reload(context.Background()); err != nil {
				log.Printf("warn: config reload rejected: %v", err)
				continue
			}
			log.Printf("info: config reloaded")
		}
	}
}

func (s *State) addWatchedFiles(watcher *fsnotify.Watcher) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.res == nil {
		return
	}
	seen := map[string]struct{}{}
	for _, layer := range s.res.Layers {
		if layer.Path == "" {
			continue
		}
		abs := layer.Path
		if !filepath.IsAbs(abs) {
			if resolved, err := filepath.Abs(abs); err == nil {
				abs = resolved
			}
		}
		if _, ok := seen[abs]; ok {
			continue
		}
		seen[abs] = struct{}{}
		_ = watcher.Add(abs)
	}
}

func shouldReloadEvent(name string, op fsnotify.Op) bool {
	if op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename) == 0 {
		return false
	}
	base := strings.ToLower(filepath.Base(name))
	return base == "kave.yaml" || base == "kave.yml" || strings.HasSuffix(base, ".yaml") || strings.HasSuffix(base, ".yml")
}
