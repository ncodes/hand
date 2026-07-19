package browser

import (
	"errors"
	"path/filepath"
	"sync"

	"github.com/gofrs/flock"
)

func getProfileLockPath(directory string) string {
	return filepath.Join(directory, ".morph-browser.lock")
}

type profileLease struct {
	lock *flock.Flock
	once sync.Once
	err  error
}

func acquireProfileLease(directory string) (*profileLease, error) {
	path := getProfileLockPath(directory)
	lock := flock.New(path)
	locked, err := lock.TryLock()
	if err != nil {
		return nil, err
	}
	if !locked {
		return nil, errors.New("browser profile is already in use")
	}

	return &profileLease{lock: lock}, nil
}

func (l *profileLease) Close() error {
	if l == nil {
		return nil
	}
	l.once.Do(func() {
		if l.lock != nil {
			l.err = l.lock.Unlock()
		}
	})

	return l.err
}
