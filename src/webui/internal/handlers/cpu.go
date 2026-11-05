package handlers

import (
	"fmt"
	"strings"
)

type cpuJSON struct {
	Cluster []cpuClusterJSON `json:"cluster"`
}

type cpuClusterJSON struct {
	ID           int    `json:"id"`
	Model        string `json:"model"`
	Cores        int    `json:"cores"`
	Frequency    int    `json:"frequency"`
	MinFrequency int    `json:"min-frequency"`
	MaxFrequency int    `json:"max-frequency"`
}

// cpuInfo is the cpu component for display.
type cpuInfo struct {
	SoC      string   // Rockchip RK3568
	Summary  string   // Rockchip RK3568, quad core Cortex-A55 @ 1992 MHz
	Clusters []string // 4x Cortex-A55 @ 1992 MHz (408-1992 MHz)
}

var coreCount = map[int]string{1: "single", 2: "dual", 4: "quad", 6: "hexa", 8: "octa"}

// cpuCluster formats a cluster, with word as 'quad core Cortex-A55'
// instead of '4x Cortex-A55', and ranges with the clock range.
func cpuCluster(cl cpuClusterJSON, word, ranges bool) string {
	text := fmt.Sprintf("%dx %s", cl.Cores, cl.Model)
	if w, ok := coreCount[cl.Cores]; ok && word {
		text = strings.TrimSpace(w + " core " + cl.Model)
	} else if cl.Model == "" {
		text = fmt.Sprintf("%d cores", cl.Cores)
		if cl.Cores == 1 {
			text = "1 core"
		}
	}
	if cl.Frequency > 0 {
		text += fmt.Sprintf(" @ %d MHz", cl.Frequency)
	}
	if ranges && cl.MinFrequency > 0 && cl.MaxFrequency > 0 {
		text += fmt.Sprintf(" (%d-%d MHz)", cl.MinFrequency, cl.MaxFrequency)
	}
	return text
}

func summarizeCPU(c hwComponentJSON) cpuInfo {
	info := cpuInfo{SoC: strings.TrimSpace(c.MfgName + " " + c.ModelName)}
	if c.CPU == nil {
		info.Summary = info.SoC
		return info
	}

	var terms []string
	for _, cl := range c.CPU.Cluster {
		info.Clusters = append(info.Clusters, cpuCluster(cl, false, true))
		terms = append(terms, cpuCluster(cl, len(c.CPU.Cluster) == 1, false))
	}
	parts := []string{}
	for _, p := range []string{info.SoC, strings.Join(terms, " + ")} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	info.Summary = capitalize(strings.Join(parts, ", "))
	return info
}
