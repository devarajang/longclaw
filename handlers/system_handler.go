package handlers

import (
	"encoding/json"
	"fmt"
	"log"
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

	// Each lookup can fail (e.g. in containers); a failed section is logged and left empty rather than
	// dereferencing a nil result.
	systemInfo.Arch = runtime.GOARCH
	systemInfo.ProcUsage.Cores = runtime.NumCPU()

	//host
	if hostInfo, err := host.Info(); err == nil {
		systemInfo.Hostname = hostInfo.Hostname
		systemInfo.OS = fmt.Sprintf("%s %s", hostInfo.Platform, hostInfo.PlatformVersion)
		systemInfo.Kernel = hostInfo.KernelVersion

		// Uptime
		uptime := time.Duration(hostInfo.Uptime) * time.Second
		systemInfo.SysUptime.Days = int(uptime.Hours()) / 24
		systemInfo.SysUptime.Hours = int(uptime.Hours()) % 24
		systemInfo.SysUptime.Minutes = int(uptime.Minutes()) % 60
	} else {
		log.Println("System info: host lookup failed:", err)
	}

	// RAM
	if vm, err := mem.VirtualMemory(); err == nil {
		systemInfo.MemoryUsage.Total = float64(vm.Total) / (1024 * 1024 * 1024)
		systemInfo.MemoryUsage.Used = float64(vm.Used) / (1024 * 1024 * 1024)
		systemInfo.MemoryUsage.Unit = "GB"
	} else {
		log.Println("System info: memory lookup failed:", err)
	}

	// Disk
	if diskUsage, err := disk.Usage("/"); err == nil {
		systemInfo.DiskUsage.Total = float64(diskUsage.Total) / (1024 * 1024 * 1024)
		systemInfo.DiskUsage.Used = float64(diskUsage.Used) / (1024 * 1024 * 1024)
		systemInfo.DiskUsage.Unit = "GB"
	} else {
		log.Println("System info: disk lookup failed:", err)
	}

	// CPU
	if cpuInfo, err := cpu.Info(); err == nil && len(cpuInfo) > 0 {
		systemInfo.ProcUsage.Model = cpuInfo[0].ModelName
	} else {
		log.Println("System info: CPU model lookup failed:", err)
	}
	if cpuPercent, err := cpu.Percent(time.Second, false); err == nil && len(cpuPercent) > 0 {
		systemInfo.ProcUsage.Usage = cpuPercent[0]
	} else {
		log.Println("System info: CPU usage lookup failed:", err)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(systemInfo)
}
