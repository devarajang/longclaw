package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"time"

	"github.com/devarajang/longclaw/dtos"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
)

func (h *Handlers) SystemInfo(w http.ResponseWriter, r *http.Request) {
	var systemInfo = dtos.SystemInfo{}

	//host
	hostInfo, _ := host.Info()

	systemInfo.Hostname = hostInfo.Hostname
	systemInfo.OS = fmt.Sprintf("%s %s", hostInfo.Platform, hostInfo.PlatformVersion)
	systemInfo.Kernel = hostInfo.KernelVersion
	systemInfo.Arch = runtime.GOARCH

	// RAM
	vm, _ := mem.VirtualMemory()
	systemInfo.MemoryUsage.Total = float64(vm.Total) / (1024 * 1024 * 1024)
	systemInfo.MemoryUsage.Used = float64(vm.Used) / (1024 * 1024 * 1024)
	systemInfo.MemoryUsage.Unit = "GB"

	// Disk
	disk, _ := disk.Usage("/")
	systemInfo.DiskUsage.Total = float64(disk.Total) / (1024 * 1024 * 1024)
	systemInfo.DiskUsage.Used = float64(disk.Used) / (1024 * 1024 * 1024)
	systemInfo.DiskUsage.Unit = "GB"

	// CPU
	cpuInfo, _ := cpu.Info()
	cpuPercent, _ := cpu.Percent(time.Second, false)

	systemInfo.ProcUsage.Model = cpuInfo[0].ModelName
	systemInfo.ProcUsage.Cores = runtime.NumCPU()
	systemInfo.ProcUsage.Usage = cpuPercent[0]

	// Uptime
	uptime := time.Duration(hostInfo.Uptime) * time.Second
	systemInfo.SysUptime.Days = int(uptime.Hours()) / 24
	systemInfo.SysUptime.Hours = int(uptime.Hours()) % 24
	systemInfo.SysUptime.Minutes = int(uptime.Minutes()) % 60

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(systemInfo)
}
