package dtos

type SystemInfo struct {
	Hostname    string `json:"hostname"`
	OS          string `json:"os"`
	Kernel      string `json:"kernel"`
	Arch        string `json:"arch"`
	MemoryUsage RAM    `json:"memory_usage"`
	DiskUsage   Disk   `json:"disk_usage"`
	ProcUsage   CPU    `json:"proc_usage"`
	SysUptime   Uptime `json:"sys_uptime"`
}

type RAM struct {
	Total float64 `json:"total"`
	Used  float64 `json:"used"`
	Unit  string  `json:"unit"`
}

type Disk struct {
	Total float64 `json:"total"`
	Used  float64 `json:"used"`
	Unit  string  `json:"unit"`
}

type CPU struct {
	Model string  `json:"model"`
	Cores int     `json:"cores"`
	Usage float64 `json:"usage"`
}

type Uptime struct {
	Days    int `json:"days"`
	Hours   int `json:"hours"`
	Minutes int `json:"minutes"`
}
