//go:build !windows

package main

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

// Non-Windows (development builds): CPU and RAM from /proc where present; everything else is
// reported as unavailable.
type sysSampler struct {
	idle, total uint64
	have        bool
}

func (s *sysSampler) fill(p *PerfSample) {
	if f, err := os.Open("/proc/stat"); err == nil {
		sc := bufio.NewScanner(f)
		if sc.Scan() {
			fs := strings.Fields(sc.Text())
			var total, idle uint64
			for i, v := range fs[1:] {
				n, _ := strconv.ParseUint(v, 10, 64)
				total += n
				if i == 3 || i == 4 {
					idle += n
				}
			}
			if s.have && total > s.total {
				p.CPU = fptr(100 * float64((total-s.total)-(idle-s.idle)) / float64(total-s.total))
			}
			s.idle, s.total, s.have = idle, total, true
		}
		f.Close()
	}
	if data, err := os.ReadFile("/proc/meminfo"); err == nil {
		var total, avail float64
		for _, l := range strings.Split(string(data), "\n") {
			fs := strings.Fields(l)
			if len(fs) >= 2 {
				v, _ := strconv.ParseFloat(fs[1], 64)
				switch fs[0] {
				case "MemTotal:":
					total = v / 1024
				case "MemAvailable:":
					avail = v / 1024
				}
			}
		}
		if total > 0 {
			p.RAMMB, p.RAMUsedMB = fptr(total), fptr(total-avail)
		}
	}
}
