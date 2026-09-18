package system

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

var packageUIDPattern = regexp.MustCompile(`^package:(\S+)\s+uid:(\d+)\s*$`)
var netstatsIdentityPattern = regexp.MustCompile(`\buid=(-?\d+)\b.*\btag=0x([0-9a-fA-F]+)\b`)
var netstatsBucketPattern = regexp.MustCompile(`\brb=(\d+)\b.*\btb=(\d+)\b`)

func ParsePackageUID(output, packageName string) (int, error) {
	for _, line := range strings.Split(output, "\n") {
		match := packageUIDPattern.FindStringSubmatch(strings.TrimSpace(line))
		if len(match) != 3 || match[1] != packageName {
			continue
		}
		uid, err := strconv.Atoi(match[2])
		if err == nil {
			return uid, nil
		}
	}
	return 0, fmt.Errorf("package UID unavailable")
}

func ParseUIDNetstats(output string, uid int) (NetInfo, error) {
	var result NetInfo
	active := false
	found := false
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if identity := netstatsIdentityPattern.FindStringSubmatch(trimmed); len(identity) == 3 {
			value, _ := strconv.Atoi(identity[1])
			active = value == uid && identity[2] == "0"
			continue
		}
		if !active {
			continue
		}
		bucket := netstatsBucketPattern.FindStringSubmatch(trimmed)
		if len(bucket) != 3 {
			continue
		}
		rx, rxErr := strconv.ParseInt(bucket[1], 10, 64)
		tx, txErr := strconv.ParseInt(bucket[2], 10, 64)
		if rxErr != nil || txErr != nil {
			continue
		}
		result.Rx += rx
		result.Tx += tx
		found = true
	}
	if !found {
		return NetInfo{}, fmt.Errorf("uid network stats unavailable")
	}
	result.Total = result.Rx + result.Tx
	result.UID, result.Scope, result.Source = uid, "package_uid", "dumpsys-netstats"
	return result, nil
}

func parseQTagUIDStats(output string, uid int) (NetInfo, error) {
	var result NetInfo
	found := false
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 8 || fields[0] == "idx" || fields[2] != "0x0" {
			continue
		}
		value, err := strconv.Atoi(fields[3])
		if err != nil || value != uid {
			continue
		}
		rx, rxErr := strconv.ParseInt(fields[5], 10, 64)
		tx, txErr := strconv.ParseInt(fields[7], 10, 64)
		if rxErr != nil || txErr != nil {
			continue
		}
		result.Rx += rx
		result.Tx += tx
		found = true
	}
	if !found {
		return NetInfo{}, fmt.Errorf("uid qtaguid stats unavailable")
	}
	result.Total = result.Rx + result.Tx
	result.UID, result.Scope, result.Source = uid, "package_uid", "xt_qtaguid"
	return result, nil
}

func (s *Service) NetworkForPackage(ctx context.Context, packageName string) (NetInfo, error) {
	output, err := s.probe(ctx, "pm", "list", "packages", "-U", packageName)
	if err != nil {
		return NetInfo{}, err
	}
	uid, err := ParsePackageUID(output, packageName)
	if err != nil {
		return NetInfo{}, err
	}
	if data, readErr := os.ReadFile("/proc/net/xt_qtaguid/stats"); readErr == nil {
		if value, parseErr := parseQTagUIDStats(string(data), uid); parseErr == nil {
			return value, nil
		}
	}
	output, err = s.probe(ctx, "dumpsys", "netstats", "detail")
	if err != nil {
		return NetInfo{}, err
	}
	return ParseUIDNetstats(output, uid)
}
