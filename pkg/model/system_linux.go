package model

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

// Memory is the memory of this machine, in bytes, 0 when unknown.
func Memory() int64 {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0
	}
	defer func() { _ = file.Close() }()
	lines := bufio.NewScanner(file)
	for lines.Scan() {
		fields := strings.Fields(lines.Text())
		if len(fields) >= 2 && fields[0] == "MemTotal:" {
			kb, _ := strconv.ParseInt(fields[1], 10, 64)
			return kb << 10
		}
	}
	return 0
}

func fastCores() int { return logicalHalf() }
