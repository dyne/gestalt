package temporal

import (
	"sync"
	"time"
)

type DevServerStatus struct {
	PID          int
	Running      bool
	LastExitTime time.Time
	LastExitErr  string
}

var devServerStatus struct {
	mu     sync.RWMutex
	status DevServerStatus
}

func DevServerStatusSnapshot() DevServerStatus {
	devServerStatus.mu.RLock()
	defer devServerStatus.mu.RUnlock()
	return devServerStatus.status
}

func UpdateDevServerStatus(update func(*DevServerStatus)) {
	if update == nil {
		return
	}
	devServerStatus.mu.Lock()
	update(&devServerStatus.status)
	devServerStatus.mu.Unlock()
}

func SetDevServerStatus(status DevServerStatus) {
	devServerStatus.mu.Lock()
	devServerStatus.status = status
	devServerStatus.mu.Unlock()
}

func ClearDevServerStatus() {
	devServerStatus.mu.Lock()
	devServerStatus.status = DevServerStatus{}
	devServerStatus.mu.Unlock()
}
