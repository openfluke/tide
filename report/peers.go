package report

import (
	"strconv"
	"strings"
)

// TidePortForCam maps test53 cam × LR band to host Tide port (container listens on 8080).
func TidePortForCam(cam int, band string) int {
	if cam < 1 {
		cam = 1
	}
	off := 0
	switch strings.ToLower(strings.TrimSpace(band)) {
	case "neg", "negative":
		off = 1
	case "hi", "high", "extreme":
		off = 2
	}
	return 8080 + (cam-1)*10 + off
}

// ParsePeerBand reads lo/hi/neg from peer names like cam1-lo, m5_hi.
func ParsePeerBand(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	for _, sep := range []string{"-", "_"} {
		for _, b := range []string{"lo", "hi", "neg"} {
			if strings.HasSuffix(name, sep+b) {
				return b
			}
		}
	}
	return ""
}

// ParsePeerCam reads N from machine/peer prefixes cam1, cam3 (not "camera").
func ParsePeerCam(name string) int {
	name = strings.ToLower(strings.TrimSpace(name))
	if !strings.HasPrefix(name, "cam") {
		return 0
	}
	rest := strings.TrimPrefix(name, "cam")
	n, err := strconv.Atoi(rest)
	if err != nil || n < 1 {
		return 0
	}
	return n
}

// CamPeerName builds cam1-lo style labels for ocean peers.
func CamPeerName(cam int, band string) string {
	if cam < 1 {
		cam = 1
	}
	band = strings.ToLower(strings.TrimSpace(band))
	if band == "" {
		band = "lo"
	}
	return "cam" + strconv.Itoa(cam) + "-" + band
}
