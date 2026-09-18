package autopopup

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestConfigKeepsLegacyFourFieldContract(t *testing.T) {
	typeOfConfig := reflect.TypeOf(Config{})
	want := []string{"autoClickByText", "autoClickByResourceId", "autoInputByHint", "autoInputByResourceId"}
	if typeOfConfig.NumField() != len(want) {
		t.Fatalf("config fields=%d, want=%d", typeOfConfig.NumField(), len(want))
	}
	for index, jsonName := range want {
		if got := strings.Split(typeOfConfig.Field(index).Tag.Get("json"), ",")[0]; got != jsonName {
			t.Fatalf("field %d json=%q, want=%q", index, got, jsonName)
		}
	}
}

type fakeHierarchy string

func (f fakeHierarchy) Hierarchy(context.Context) (string, error)         { return string(f), nil }
func (f fakeHierarchy) ForegroundPackage(context.Context) (string, error) { return "com.example", nil }

type fakeExecutor struct{ commands []string }

func (f *fakeExecutor) Run(_ context.Context, name string, args ...string) (string, error) {
	f.commands = append(f.commands, name+" "+strings.Join(args, " "))
	return "", nil
}
func (*fakeExecutor) RunBytes(context.Context, string, ...string) ([]byte, error) {
	return nil, fmt.Errorf("unused")
}
func TestScanClicksConfiguredNode(t *testing.T) {
	e := &fakeExecutor{}
	device := fakeHierarchy(`<hierarchy><node package="com.example" text="关闭" clickable="true" enabled="true" visible-to-user="true" bounds="[10,20][110,220]"/></hierarchy>`)
	m := New(device, e, func() map[string]any {
		return map[string]any{"autoClickByText": []any{map[string]any{"clickTarget": "关闭"}}}
	})
	if err := m.scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(e.commands) != 1 || e.commands[0] != "input tap 60 120" {
		t.Fatalf("commands=%v", e.commands)
	}
}

func TestScanPreservesExplicitLegacyAction(t *testing.T) {
	e := &fakeExecutor{}
	device := fakeHierarchy(`<hierarchy><node package="com.example" text="开始试用" clickable="false" enabled="true" visible-to-user="false" bounds="[10,20][110,220]"/></hierarchy>`)
	m := New(device, e, func() map[string]any {
		return map[string]any{"autoClickByText": []any{map[string]any{"clickTarget": "开始试用"}}}
	})
	if err := m.scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(e.commands) != 1 || e.commands[0] != "input tap 60 120" {
		t.Fatalf("explicit legacy action was not executed: %v", e.commands)
	}
}

func TestScanRequiresConfiguredMainContentContext(t *testing.T) {
	e := &fakeExecutor{}
	device := fakeHierarchy(`<hierarchy><node package="com.example" text="关闭" enabled="true" bounds="[10,20][110,220]"/></hierarchy>`)
	m := New(device, e, func() map[string]any {
		return map[string]any{"autoClickByText": []any{map[string]any{"mainContentContains": "升级提示", "clickTarget": "关闭"}}}
	})
	if err := m.scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(e.commands) != 0 {
		t.Fatalf("context-free action executed: %v", e.commands)
	}
}
func TestCenterRejectsInvalidBounds(t *testing.T) {
	if _, _, ok := center("invalid"); ok {
		t.Fatal("invalid bounds accepted")
	}
}
