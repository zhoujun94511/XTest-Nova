package command

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var packageNamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*(?:\.[A-Za-z][A-Za-z0-9_]*)+$`)

func parseMonkey(args []string) (Invocation, error) {
	jsonOutput, rest, err := splitGlobalFlags(args[1:])
	if err != nil {
		return Invocation{}, err
	}
	if len(rest) == 0 {
		return Invocation{}, fmt.Errorf("usage: xtest-nova-agent monkey start|status|stop|report")
	}
	action := strings.ToLower(rest[0])
	flags, positionals, err := parseFlags(rest[1:])
	if err != nil {
		return Invocation{}, err
	}
	invocation := Invocation{Kind: Monkey, JSON: jsonOutput, MonkeyAction: action}
	switch action {
	case "status", "stop":
		if len(positionals) != 0 {
			return Invocation{}, fmt.Errorf("usage: xtest-nova-agent monkey %s [--engine runner|exploration]", action)
		}
		engine, err := takeEngine(flags)
		if err != nil {
			return Invocation{}, err
		}
		if len(flags) != 0 {
			return Invocation{}, fmt.Errorf("usage: xtest-nova-agent monkey %s [--engine runner|exploration]", action)
		}
		invocation.Engine = engine
		return invocation, nil
	case "report":
		if len(positionals) != 0 || len(flags) != 0 {
			return Invocation{}, fmt.Errorf("usage: xtest-nova-agent monkey %s", action)
		}
		return invocation, nil
	case "start":
		if len(positionals) != 0 {
			return Invocation{}, fmt.Errorf("usage: xtest-nova-agent monkey start --package <package> --duration <seconds>")
		}
		if err := applyTaskFlags(&invocation, flags, true, false); err != nil {
			return Invocation{}, err
		}
		return invocation, nil
	default:
		return Invocation{}, fmt.Errorf("unsupported monkey action %q", rest[0])
	}
}

func parseExtended(args []string) (Invocation, error) {
	jsonOutput, rest, err := splitGlobalFlags(args)
	if err != nil {
		return Invocation{}, err
	}
	if len(rest) == 0 {
		return Invocation{}, fmt.Errorf("unknown command")
	}
	flags, positionals, err := parseFlags(rest[1:])
	if err != nil {
		return Invocation{}, err
	}
	invocation := Invocation{JSON: jsonOutput}
	switch rest[0] {
	case "doctor":
		return finish(invocation, Doctor, flags, positionals)
	case "device":
		return finish(invocation, DeviceInfo, flags, positionals)
	case "foreground":
		return finish(invocation, Foreground, flags, positionals)
	case "apps":
		if len(positionals) == 0 {
			return Invocation{}, fmt.Errorf("usage: xtest-nova-agent apps list|info <package>|launch <package>")
		}
		invocation.Kind = Apps
		invocation.Action = strings.ToLower(positionals[0])
		switch invocation.Action {
		case "list":
			if len(positionals) != 1 || len(flags) != 0 {
				return Invocation{}, fmt.Errorf("usage: xtest-nova-agent apps list")
			}
		case "info", "launch":
			if len(positionals) != 2 || len(flags) != 0 {
				return Invocation{}, fmt.Errorf("usage: xtest-nova-agent apps %s <package>", invocation.Action)
			}
			invocation.PackageName = positionals[1]
			if err := requirePackage(invocation.PackageName); err != nil {
				return Invocation{}, err
			}
		default:
			return Invocation{}, fmt.Errorf("unsupported apps action %q", positionals[0])
		}
		return invocation, nil
	case "screenshot":
		if len(positionals) != 1 || len(flags) != 0 {
			return Invocation{}, fmt.Errorf("usage: xtest-nova-agent screenshot <file>")
		}
		invocation.Kind = ScreenshotCmd
		target, err := allowedDevicePath(positionals[0])
		if err != nil {
			return Invocation{}, err
		}
		invocation.TargetPath = target
		return invocation, nil
	case "explore":
		return parseExplore(invocation, flags, positionals)
	case "record":
		return parseRecord(invocation, flags, positionals)
	case "replay":
		return parseReplay(invocation, flags, positionals)
	case "perf":
		return parsePerf(invocation, flags, positionals)
	case "artifacts":
		invocation.Kind = Artifacts
		if len(positionals) != 0 {
			return Invocation{}, fmt.Errorf("usage: xtest-nova-agent artifacts [--package <package>]")
		}
		if err := applyOptionalPackage(&invocation, flags); err != nil {
			return Invocation{}, err
		}
		return invocation, nil
	case "pull":
		if len(positionals) != 2 || len(flags) != 0 {
			return Invocation{}, fmt.Errorf("usage: xtest-nova-agent pull <directory> <file>")
		}
		invocation.Kind = Pull
		source, err := allowedDevicePath(positionals[0])
		if err != nil {
			return Invocation{}, err
		}
		target, err := allowedDevicePath(positionals[1])
		if err != nil {
			return Invocation{}, err
		}
		invocation.TargetPath = source
		invocation.DestPath = target
		return invocation, nil
	default:
		return Invocation{}, fmt.Errorf("unknown command %q", rest[0])
	}
}

func parseExplore(invocation Invocation, flags map[string]string, positionals []string) (Invocation, error) {
	if len(positionals) == 0 {
		return Invocation{}, fmt.Errorf("usage: xtest-nova-agent explore preview|start|status|stop")
	}
	invocation.Kind = Explore
	invocation.Action = strings.ToLower(positionals[0])
	if len(positionals) != 1 {
		return Invocation{}, fmt.Errorf("usage: xtest-nova-agent explore %s", invocation.Action)
	}
	switch invocation.Action {
	case "preview":
		if err := applyTaskFlags(&invocation, flags, true, false); err != nil {
			return Invocation{}, err
		}
	case "start":
		if err := applyTaskFlags(&invocation, flags, true, true); err != nil {
			return Invocation{}, err
		}
	case "status", "stop":
		if len(flags) != 0 {
			return Invocation{}, fmt.Errorf("usage: xtest-nova-agent explore %s", invocation.Action)
		}
	default:
		return Invocation{}, fmt.Errorf("unsupported explore action %q", positionals[0])
	}
	return invocation, nil
}

func parseRecord(invocation Invocation, flags map[string]string, positionals []string) (Invocation, error) {
	if len(positionals) == 0 {
		return Invocation{}, fmt.Errorf("usage: xtest-nova-agent record start|status|stop|cases")
	}
	invocation.Kind = Record
	invocation.Action = strings.ToLower(positionals[0])
	if len(positionals) != 1 {
		return Invocation{}, fmt.Errorf("usage: xtest-nova-agent record %s", invocation.Action)
	}
	switch invocation.Action {
	case "start":
		if err := applyTaskFlags(&invocation, flags, true, false); err != nil {
			return Invocation{}, err
		}
		if invocation.Name == "" {
			return Invocation{}, fmt.Errorf("--name is required")
		}
	case "cases":
		if err := applyOptionalPackage(&invocation, flags); err != nil {
			return Invocation{}, err
		}
	case "status", "stop":
		if len(flags) != 0 {
			return Invocation{}, fmt.Errorf("usage: xtest-nova-agent record %s", invocation.Action)
		}
	default:
		return Invocation{}, fmt.Errorf("unsupported record action %q", positionals[0])
	}
	return invocation, nil
}

func parseReplay(invocation Invocation, flags map[string]string, positionals []string) (Invocation, error) {
	if len(positionals) == 0 {
		return Invocation{}, fmt.Errorf("usage: xtest-nova-agent replay start|status|stop")
	}
	invocation.Kind = Replay
	invocation.Action = strings.ToLower(positionals[0])
	if len(positionals) != 1 {
		return Invocation{}, fmt.Errorf("usage: xtest-nova-agent replay %s", invocation.Action)
	}
	switch invocation.Action {
	case "start":
		if err := applyTaskFlags(&invocation, flags, true, true); err != nil {
			return Invocation{}, err
		}
		if invocation.Name == "" {
			return Invocation{}, fmt.Errorf("--name is required")
		}
	case "status", "stop":
		if len(flags) != 0 {
			return Invocation{}, fmt.Errorf("usage: xtest-nova-agent replay %s", invocation.Action)
		}
	default:
		return Invocation{}, fmt.Errorf("unsupported replay action %q", positionals[0])
	}
	return invocation, nil
}

func parsePerf(invocation Invocation, flags map[string]string, positionals []string) (Invocation, error) {
	if len(positionals) == 0 {
		return Invocation{}, fmt.Errorf("usage: xtest-nova-agent perf start|status|stop")
	}
	invocation.Kind = Perf
	invocation.Action = strings.ToLower(positionals[0])
	if len(positionals) != 1 {
		return Invocation{}, fmt.Errorf("usage: xtest-nova-agent perf %s", invocation.Action)
	}
	switch invocation.Action {
	case "start":
		if err := applyTaskFlags(&invocation, flags, true, false); err != nil {
			return Invocation{}, err
		}
	case "status", "stop":
		if len(flags) != 0 {
			return Invocation{}, fmt.Errorf("usage: xtest-nova-agent perf %s", invocation.Action)
		}
	default:
		return Invocation{}, fmt.Errorf("unsupported perf action %q", positionals[0])
	}
	return invocation, nil
}

func finish(invocation Invocation, kind Kind, flags map[string]string, positionals []string) (Invocation, error) {
	if len(flags) != 0 || len(positionals) != 0 {
		return Invocation{}, fmt.Errorf("usage: xtest-nova-agent %s", commandName(kind))
	}
	invocation.Kind = kind
	return invocation, nil
}

func commandName(kind Kind) string {
	switch kind {
	case Doctor:
		return "doctor"
	case DeviceInfo:
		return "device"
	case Foreground:
		return "foreground"
	default:
		return "command"
	}
}

func splitGlobalFlags(args []string) (bool, []string, error) {
	jsonOutput := false
	rest := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == "--json" {
			jsonOutput = true
			continue
		}
		rest = append(rest, arg)
	}
	return jsonOutput, rest, nil
}

func parseFlags(args []string) (map[string]string, []string, error) {
	flags := map[string]string{}
	positionals := make([]string, 0)
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if arg == "--" {
			positionals = append(positionals, args[index+1:]...)
			break
		}
		if !strings.HasPrefix(arg, "--") {
			positionals = append(positionals, arg)
			continue
		}
		name := strings.TrimPrefix(arg, "--")
		switch name {
		case "execute", "low-battery":
			flags[name] = "true"
		case "package", "duration", "name", "task", "engine", "throttle", "render", "min-battery", "activity-mode", "activities", "control-blacklist", "target-cases":
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "--") {
				return nil, nil, fmt.Errorf("--%s requires a value", name)
			}
			index++
			flags[name] = args[index]
		default:
			return nil, nil, fmt.Errorf("unknown flag %q", arg)
		}
	}
	return flags, positionals, nil
}

func applyTaskFlags(invocation *Invocation, flags map[string]string, packageRequired, allowExecute bool) error {
	for name, value := range flags {
		switch name {
		case "package":
			invocation.PackageName = value
		case "duration":
			if invocation.Kind != Monkey && invocation.Kind != Explore {
				return fmt.Errorf("unknown flag \"--duration\"")
			}
			seconds, err := strconv.Atoi(value)
			if err != nil || seconds < 1 || seconds > 604800 {
				return fmt.Errorf("--duration must be between 1 and 604800 seconds")
			}
			if seconds < 60 {
				seconds = 60
			}
			invocation.DurationSeconds = seconds
		case "name":
			if invocation.Kind != Record && invocation.Kind != Replay {
				return fmt.Errorf("unknown flag \"--name\"")
			}
			invocation.Name = value
		case "task":
			if invocation.Kind != Record {
				return fmt.Errorf("unknown flag \"--task\"")
			}
			invocation.Task = value
		case "execute":
			if !allowExecute {
				return fmt.Errorf("unknown flag \"--execute\"")
			}
			invocation.Execute = true
		case "engine", "throttle", "render", "min-battery", "activity-mode", "activities", "control-blacklist", "target-cases", "low-battery":
			starting := (invocation.Kind == Monkey && invocation.MonkeyAction == "start") || (invocation.Kind == Explore && invocation.Action == "start")
			if !starting {
				return fmt.Errorf("unknown flag \"--%s\"", name)
			}
		default:
			return fmt.Errorf("unknown flag \"--%s\"", name)
		}
	}
	if (invocation.Kind == Monkey && invocation.MonkeyAction == "start") || (invocation.Kind == Explore && invocation.Action == "start") {
		if err := applyTraversalFlags(invocation, flags); err != nil {
			return err
		}
	}
	if packageRequired {
		if err := requirePackage(invocation.PackageName); err != nil {
			return err
		}
	}
	if ((invocation.Kind == Monkey && invocation.MonkeyAction == "start") || (invocation.Kind == Explore && invocation.Action == "start")) && invocation.DurationSeconds < 60 {
		invocation.DurationSeconds = 60
	}
	return nil
}

func takeEngine(flags map[string]string) (string, error) {
	value, ok := flags["engine"]
	if !ok {
		return "", nil
	}
	delete(flags, "engine")
	switch value {
	case "runner", "exploration":
		return value, nil
	default:
		return "", fmt.Errorf("--engine must be runner or exploration")
	}
}

func applyTraversalFlags(invocation *Invocation, flags map[string]string) error {
	invocation.ThrottleMillis = 500
	invocation.RenderFallbackMode = "bounded"
	invocation.LowBatteryExit = true
	invocation.MinBattery = 15
	invocation.ActivityMode = "none"
	if invocation.Kind == Monkey {
		engine, err := takeEngine(flags)
		if err != nil {
			return err
		}
		invocation.Engine = engine
	} else if _, ok := flags["engine"]; ok {
		return fmt.Errorf("unknown flag \"--engine\"")
	}
	minThrottle := 100
	if invocation.Kind == Monkey && invocation.Engine == "runner" {
		minThrottle = 50
	}
	if value, ok := flags["throttle"]; ok {
		millis, err := strconv.Atoi(value)
		if err != nil || millis < minThrottle || millis > 60000 {
			return fmt.Errorf("--throttle must be between %d and 60000 milliseconds", minThrottle)
		}
		invocation.ThrottleMillis = millis
	}
	if value, ok := flags["render"]; ok {
		switch value {
		case "off", "bounded", "continuous":
			invocation.RenderFallbackMode = value
		default:
			return fmt.Errorf("--render must be off, bounded, or continuous")
		}
	}
	if value, ok := flags["min-battery"]; ok {
		percent, err := strconv.Atoi(value)
		if err != nil || percent < 0 || percent > 100 {
			return fmt.Errorf("--min-battery must be 0 or between 1 and 100")
		}
		invocation.LowBatteryExit = percent > 0
		invocation.MinBattery = percent
	}
	if flags["low-battery"] == "true" {
		invocation.LowBatteryExit = true
		if invocation.MinBattery == 0 {
			invocation.MinBattery = 15
		}
	}
	if value, ok := flags["activity-mode"]; ok {
		switch value {
		case "none", "allowlist", "blocklist":
			invocation.ActivityMode = value
		default:
			return fmt.Errorf("--activity-mode must be none, allowlist, or blocklist")
		}
	}
	invocation.Activities = splitList(flags["activities"])
	invocation.ControlBlacklist = splitList(flags["control-blacklist"])
	cases, err := parseTargetCases(flags["target-cases"])
	if err != nil {
		return err
	}
	invocation.TargetCases = cases
	if (invocation.ActivityMode == "allowlist" || invocation.ActivityMode == "blocklist") && len(invocation.Activities) == 0 {
		return fmt.Errorf("--activities is required when --activity-mode is %s", invocation.ActivityMode)
	}
	return nil
}

func splitList(value string) []string {
	if strings.TrimSpace(value) == "" {
		return []string{}
	}
	parts := strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == '，' })
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			items = append(items, part)
		}
	}
	return items
}

func parseTargetCases(value string) ([]TargetCase, error) {
	if strings.TrimSpace(value) == "" {
		return []TargetCase{}, nil
	}
	parts := strings.FieldsFunc(value, func(r rune) bool { return r == ';' || r == '；' })
	cases := make([]TargetCase, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		equals := strings.Index(part, "=")
		slash := strings.LastIndex(part, "/")
		if equals < 1 || slash <= equals+1 || slash == len(part)-1 {
			return nil, fmt.Errorf("--target-cases entries must use Activity=任务/用例")
		}
		cases = append(cases, TargetCase{Activity: strings.TrimSpace(part[:equals]), Task: strings.TrimSpace(part[equals+1 : slash]), Case: strings.TrimSpace(part[slash+1:])})
	}
	return cases, nil
}

func applyOptionalPackage(invocation *Invocation, flags map[string]string) error {
	for name, value := range flags {
		if name != "package" {
			return fmt.Errorf("unknown flag \"--%s\"", name)
		}
		invocation.PackageName = value
	}
	if invocation.PackageName == "" {
		return nil
	}
	return requirePackage(invocation.PackageName)
}

func requirePackage(value string) error {
	if !packageNamePattern.MatchString(strings.TrimSpace(value)) {
		return fmt.Errorf("--package is required and must be an Android package name")
	}
	return nil
}
