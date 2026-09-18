package touchreader

import "testing"

func TestParseDeviceAndProtocolB(t *testing.T) {
	probe := "add device 1: /dev/input/event7\n  events:\n    ABS_MT_SLOT : min 0, max 9\n    ABS_MT_POSITION_X : min 0, max 1080\n    ABS_MT_POSITION_Y : min 0, max 2400\n  input props:\n    INPUT_PROP_DIRECT\n"
	device, err := ParseDevice(probe)
	if err != nil || device.Path != "/dev/input/event7" || device.Contacts != 10 {
		t.Fatalf("device=%+v err=%v", device, err)
	}
	s := state{maxX: 1080, maxY: 2400, maxContacts: 10}
	s.apply(inputEvent{kind: evAbs, code: absTracking, value: 1})
	s.apply(inputEvent{kind: evAbs, code: absX, value: 540})
	s.apply(inputEvent{kind: evAbs, code: absY, value: 600})
	events := s.apply(inputEvent{kind: evSyn, code: synReport})
	if len(events) != 1 || events[0].Operation != "d" || events[0].XP != .5 || events[0].YP != .25 {
		t.Fatalf("events=%+v", events)
	}
}
