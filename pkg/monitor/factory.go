package monitor

import "time"

// MonitorFactory creates monitor instances using the Factory pattern
type MonitorFactory struct{}

// NewMonitorFactory creates a new monitor factory
func NewMonitorFactory() *MonitorFactory {
	return &MonitorFactory{}
}

// CreateNetworkMonitor creates a new NetworkMonitor
func (f *MonitorFactory) CreateNetworkMonitor() *NetworkMonitor {
	return NewNetworkMonitor()
}

// CreateResourceMonitor creates a new ResourceMonitor
func (f *MonitorFactory) CreateResourceMonitor() *ResourceMonitor {
	return NewResourceMonitor()
}

// CreateResourceMonitorForPID creates a new ResourceMonitor for a specific PID
func (f *MonitorFactory) CreateResourceMonitorForPID(pid int) *ResourceMonitor {
	return NewResourceMonitorForPID(pid)
}

// CreateDownloadMonitor creates a new DownloadMonitor
func (f *MonitorFactory) CreateDownloadMonitor(targetDir string) *DownloadMonitor {
	return NewDownloadMonitor(targetDir)
}

// CreateDiskWriteMonitor creates a new DiskWriteMonitor
func (f *MonitorFactory) CreateDiskWriteMonitor(targetDir string) *DiskWriteMonitor {
	return NewDiskWriteMonitor(targetDir)
}

// CreateOutputVerifier creates a new OutputVerifier
func (f *MonitorFactory) CreateOutputVerifier(directory string) *OutputVerifier {
	return NewOutputVerifier(directory)
}

// CreateMonitorSet creates a complete set of monitors for a test iteration
type MonitorSet struct {
	Network  *NetworkMonitor
	Resource *ResourceMonitor
	Download *DownloadMonitor
	Disk     *DiskWriteMonitor
}

// CreateMonitorSet creates a full set of monitors for monitoring a test iteration
func (f *MonitorFactory) CreateMonitorSet(downloadDir string) *MonitorSet {
	return &MonitorSet{
		Network:  f.CreateNetworkMonitor(),
		Resource: f.CreateResourceMonitor(),
		Download: f.CreateDownloadMonitor(downloadDir),
		Disk:     f.CreateDiskWriteMonitor(downloadDir),
	}
}

// StartAll starts all monitors in the set
func (ms *MonitorSet) StartAll() error {
	if err := ms.Network.Start(); err != nil {
		return err
	}
	if err := ms.Resource.Start(); err != nil {
		return err
	}
	if err := ms.Download.Start(); err != nil {
		return err
	}
	if err := ms.Disk.Start(); err != nil {
		return err
	}
	return nil
}

// StopAll stops all monitors in the set
func (ms *MonitorSet) StopAll() {
	ms.Network.Stop()
	ms.Resource.Stop()
	ms.Download.Stop()
	ms.Disk.Stop()
}

// SetPollInterval sets the polling interval for all polling monitors
func (ms *MonitorSet) SetPollInterval(interval time.Duration) {
	ms.Resource.SetPollInterval(interval)
	ms.Download.SetPollInterval(interval)
	ms.Disk.SetPollInterval(interval)
}




