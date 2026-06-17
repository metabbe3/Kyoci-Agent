package platform

import (
	"os"
	"runtime"
	"strings"
)

// OS represents the detected operating system.
type OS int

const (
	OSUnknown OS = iota
	OSMacOS
	OSLinux
	OSWindows
)

// String returns a human-readable OS name.
func (o OS) String() string {
	switch o {
	case OSMacOS:
		return "macOS"
	case OSLinux:
		return "Linux"
	case OSWindows:
		return "Windows"
	default:
		return "Unknown"
	}
}

// Detect returns the current operating system.
func Detect() OS {
	switch runtime.GOOS {
	case "darwin":
		return OSMacOS
	case "linux":
		return OSLinux
	case "windows":
		return OSWindows
	default:
		return OSUnknown
	}
}

// ShellPath returns the best shell for this OS.
func ShellPath() string {
	switch Detect() {
	case OSWindows:
		// Try PowerShell first, fall back to cmd
		if p := os.Getenv("SystemRoot"); p != "" {
			if _, err := os.Stat(p + "\\System32\\WindowsPowerShell\\v1.0\\powershell.exe"); err == nil {
				return p + "\\System32\\WindowsPowerShell\\v1.0\\powershell.exe"
			}
		}
		return "cmd.exe"
	default:
		// macOS + Linux: prefer bash, fall back to sh
		if _, err := os.Stat("/bin/bash"); err == nil {
			return "/bin/bash"
		}
		return "/bin/sh"
	}
}

// ShellArgs returns the shell invocation arguments for a command.
func ShellArgs() []string {
	switch Detect() {
	case OSWindows:
		return []string{"-NoProfile", "-NonInteractive", "-Command"}
	default:
		return []string{"-c"}
	}
}

// PathSeparator returns the OS-appropriate path separator.
func PathSeparator() string {
	if Detect() == OSWindows {
		return ";"
	}
	return ":"
}

// ExtraPaths returns additional PATH directories for the detected OS.
func ExtraPaths() []string {
	switch Detect() {
	case OSMacOS:
		return []string{"/usr/local/bin", "/opt/homebrew/bin", "/usr/sbin", "/sbin"}
	case OSLinux:
		return []string{"/usr/local/bin", "/usr/sbin", "/sbin", "/snap/bin"}
	case OSWindows:
		return []string{} // Windows uses PATHEXT, not extra PATH dirs
	default:
		return []string{"/usr/local/bin", "/usr/sbin", "/sbin"}
	}
}

// OpenBrowserCommand returns the command to open a URL in the default browser.
func OpenBrowserCommand(url string) string {
	switch Detect() {
	case OSMacOS:
		return "open " + quoteArg(url)
	case OSLinux:
		return "xdg-open " + quoteArg(url)
	case OSWindows:
		return "start " + quoteArg(url)
	default:
		return "open " + quoteArg(url)
	}
}

func quoteArg(s string) string {
	if strings.ContainsAny(s, " \t\"'") {
		return "\"" + strings.ReplaceAll(s, "\"", "\\\"") + "\""
	}
	return s
}

// GetCommandReference returns an OS-specific command reference table
// that gets injected into the system prompt. This is the KEY mechanism
// for making an 8B model work cross-platform: instead of hoping the model
// "knows" which commands to use, we give it an explicit lookup table.
func GetCommandReference() string {
	osType := Detect()
	switch osType {
	case OSMacOS:
		return macOSCommands
	case OSLinux:
		return linuxCommands
	case OSWindows:
		return windowsCommands
	default:
		return macOSCommands // safe default
	}
}

// GetPlatformSection returns the full platform context block for system prompts.
// This includes OS name, shell, and the command reference table.
func GetPlatformSection() string {
	osType := Detect()
	var sb strings.Builder

	sb.WriteString("SYSTEM ENVIRONMENT:\n")
	sb.WriteString("- Operating System: " + osType.String() + "\n")
	sb.WriteString("- Shell: " + ShellPath() + "\n")
	if home, err := os.UserHomeDir(); err == nil {
		sb.WriteString("- Home directory: " + home + "\n")
	}
	if cwd, err := os.Getwd(); err == nil {
		sb.WriteString("- Working directory: " + cwd + "\n")
	}
	sb.WriteString("\n")
	sb.WriteString(GetCommandReference())

	return sb.String()
}

// ============================================================================
// OS-Specific Command Reference Tables
// These are designed for 8B models: explicit, compact, no ambiguity.
// The model pattern-matches, not reasons.
// ============================================================================

const macOSCommands = `OS COMMANDS (macOS/Darwin — use these EXACT commands):
System info:    uname -a | sw_vers
CPU info:       sysctl -n machdep.cpu.brand_string
Memory:         vm_stat | top -l 1 -s 0 | ps aux --sort=-%mem | head -20
Disk space:     df -h /
Disk usage:     du -sh /path/* | sort -rh | head -20
Processes:      ps aux | top -l 1 -s 0
Network:        ifconfig | netstat -an | lsof -i :PORT
Open port:      lsof -i :PORT
System logs:    log show --predicate '...' --last 1h
Services:       launchctl list | launchctl start NAME
Package mgr:    brew install PKG | brew list | brew update
Firewall:       sudo /usr/libexec/ApplicationFirewall/socketfilterfw --getglobalstate
DNS lookup:     dig DOMAIN | nslookup DOMAIN
SSH:            ssh user@host
SCP:            scp file user@host:/path
Cron/sched:     crontab -l | crontab -e
File search:    find /path -name "*.go" | mdfind "name:keyword"
Text search:    grep -rn "pattern" /path | rg "pattern"
Zip/unzip:      zip -r archive.zip dir/ | unzip archive.zip
TAR:            tar -czf archive.tar.gz dir/ | tar -xzf archive.tar.gz
Git:            git status | git log --oneline -10 | git diff
Docker:         docker ps | docker logs NAME | docker compose up -d
Python:         python3 script.py | pip3 install PKG
Node:           node script.js | npm install PKG
Go:             go build ./... | go test ./... | go run .

FORBIDDEN on macOS: free, dmesg, journalctl, systemctl, apt, yum, pacman, sed -i without backup file`

const linuxCommands = `OS COMMANDS (Linux — use these EXACT commands):
System info:    uname -a | cat /etc/os-release
CPU info:       lscpu | cat /proc/cpuinfo | nproc
Memory:         free -h | cat /proc/meminfo
Disk space:     df -h /
Disk usage:     du -sh /path/* | sort -rh | head -20
Processes:      ps aux | top -bn1 | htop
Network:        ip addr | ss -tlnp | netstat -tlnp
Open port:      ss -tlnp | grep :PORT
System logs:    journalctl -u SERVICE --since "1 hour ago" | dmesg | tail -f /var/log/syslog
Services:       systemctl status NAME | systemctl restart NAME
Package mgr:    apt install PKG (Debian/Ubuntu) | yum install PKG (RHEL/CentOS) | dnf install PKG (Fedora) | pacman -S PKG (Arch)
Firewall:       ufw status | iptables -L -n
DNS lookup:     dig DOMAIN | nslookup DOMAIN
SSH:            ssh user@host
SCP:            scp file user@host:/path
Cron/sched:     crontab -l | crontab -e | systemctl list-timers
File search:    find /path -name "*.go" | locate keyword
Text search:    grep -rn "pattern" /path | rg "pattern"
Zip/unzip:      zip -r archive.zip dir/ | unzip archive.zip
TAR:            tar -czf archive.tar.gz dir/ | tar -xzf archive.tar.gz
Git:            git status | git log --oneline -10 | git diff
Docker:         docker ps | docker logs NAME | docker compose up -d
Python:         python3 script.py | pip3 install PKG
Node:           node script.js | npm install PKG
Go:             go build ./... | go test ./... | go run .

FORBIDDEN on Linux: launchctl, vm_stat, top -l, brew (unless installed), system_profiler`

const windowsCommands = `OS COMMANDS (Windows/PowerShell — use these EXACT commands):
System info:    systeminfo | Get-ComputerInfo
CPU info:       Get-CimInstance Win32_Processor | Select-Object Name
Memory:         Get-CimInstance Win32_OperatingSystem | Select-Object FreePhysicalMemory,TotalVisibleMemorySize
Disk space:     Get-PSDrive -PSProvider FileSystem | wmic logicaldisk get size,freespace,caption
Disk usage:     Get-ChildItem -Path C:\ -Recurse -ErrorAction SilentlyContinue | Measure-Object Length -Sum
Processes:      Get-Process | tasklist
Network:        Get-NetIPAddress | netstat -ano | Get-NetTCPConnection
Open port:      Get-NetTCPConnection -LocalPort PORT
System logs:    Get-EventLog -LogName System -Newest 20 | Get-WinEvent -FilterHashtable @{LogName='System'}
Services:       Get-Service | Start-Service NAME | Stop-Service NAME
Package mgr:    winget install PKG | choco install PKG
Firewall:       Get-NetFirewallProfile | netsh advfirewall show currentprofile
DNS lookup:     Resolve-DnsName DOMAIN | nslookup DOMAIN
SSH:            ssh user@host
SCP:            scp file user@host:/path
Sched tasks:    Get-ScheduledTask | schtasks /query
File search:    Get-ChildItem -Path C:\ -Filter *.go -Recurse | dir /s /b *.go
Text search:    Select-String -Pattern "keyword" -Path C:\path\ -Recurse | findstr /s /i "keyword"
Zip/unzip:      Compress-Archive -Path dir\ -DestinationPath archive.zip | Expand-Archive archive.zip
TAR:            tar -czf archive.tar.gz dir\ | tar -xzf archive.tar.gz
Git:            git status | git log --oneline -10 | git diff
Docker:         docker ps | docker logs NAME | docker compose up -d
Python:         python script.py | pip install PKG
Node:           node script.js | npm install PKG
Go:             go build ./... | go test ./... | go run .
Open browser:   start https://url | Start-Process "https://url"

FORBIDDEN on Windows: bash commands like cat/grep/find/ls (use Get-Content, Select-String, Get-ChildItem), free, df, top, ps, launchctl, systemctl

IMPORTANT: On Windows, use PowerShell cmdlets (Get-*, Set-*, New-*). Pipe with |. Use Select-Object to filter columns.`
