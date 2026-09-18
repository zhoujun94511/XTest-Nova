package apps

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/zhoujun94511/xtest-nova/agent/internal/pkgmeta"
)

type fakeExecutor struct{ outputs map[string]string }

type fakeMetadata struct {
	icon []byte
	err  error
}

func (f fakeMetadata) Icon(_ context.Context, _ string) ([]byte, error) { return f.icon, f.err }
func (f fakeMetadata) Labels(_ context.Context, _ string) (pkgmeta.Labels, error) {
	return pkgmeta.Labels{Chinese: "示例应用", English: "Example"}, f.err
}
func (f fakeMetadata) Catalog(_ context.Context) (map[string]pkgmeta.Labels, error) {
	return map[string]pkgmeta.Labels{"com.example.a": {Chinese: "示例应用", English: "Example"}}, f.err
}

func (f fakeExecutor) Run(_ context.Context, name string, args ...string) (string, error) {
	key := name + " " + strings.Join(args, " ")
	value, ok := f.outputs[key]
	if !ok {
		return "", fmt.Errorf("unexpected command: %s", key)
	}
	return value, nil
}
func (fakeExecutor) RunBytes(_ context.Context, _ string, _ ...string) ([]byte, error) {
	return nil, nil
}
func TestListAndInfo(t *testing.T) {
	executor := fakeExecutor{outputs: map[string]string{
		"pm list packages -f -3":                             "package:/data/app/a/base.apk=com.example.a\npackage:/data/app/b/base.apk=com.example.b",
		"pm path com.example.a":                              "package:/data/app/a/base.apk",
		"dumpsys package com.example.a":                      "versionCode=42 minSdk=28\n versionName=1.2.3",
		"cmd package resolve-activity --brief com.example.a": "com.example.a/.MainActivity",
	}}
	service := New(executor, 0)
	list, err := service.List(context.Background(), false)
	if err != nil || len(list) != 2 {
		t.Fatalf("list=%v err=%v", list, err)
	}
	info, err := service.Info(context.Background(), "com.example.a")
	if err != nil {
		t.Fatal(err)
	}
	if info.VersionCode != 42 || info.VersionName != "1.2.3" || info.MainActivity != ".MainActivity" {
		t.Fatalf("%+v", info)
	}
}
func TestRejectInvalidPackage(t *testing.T) {
	service := New(fakeExecutor{}, 0)
	if _, err := service.Info(context.Background(), "../bad"); err == nil {
		t.Fatal("invalid package accepted")
	}
}

func TestIconUsesAndroidMetadataProvider(t *testing.T) {
	want := []byte{0xff, 0xd8, 0xff, 0xd9}
	service := New(fakeExecutor{}, time.Second, fakeMetadata{icon: want})
	got, err := service.Icon(context.Background(), "com.example.app")
	if err != nil || string(got) != string(want) {
		t.Fatalf("icon=%x err=%v", got, err)
	}
}

func TestListAndInfoPreferChineseThenEnglishAndPackageName(t *testing.T) {
	executor := fakeExecutor{outputs: map[string]string{
		"pm list packages -f -3":                             "package:/data/app/a/base.apk=com.example.a\npackage:/data/app/b/base.apk=com.example.b",
		"pm path com.example.a":                              "package:/data/app/a/base.apk",
		"dumpsys package com.example.a":                      "versionCode=1\n versionName=1",
		"cmd package resolve-activity --brief com.example.a": "com.example.a/.MainActivity",
	}}
	service := New(executor, time.Second, fakeMetadata{})
	list, err := service.List(context.Background(), false)
	byPackage := map[string]Package{}
	for _, item := range list {
		byPackage[item.PackageName] = item
	}
	if err != nil || byPackage["com.example.a"].Name != "示例应用" || byPackage["com.example.b"].Name != "com.example.b" {
		t.Fatalf("list=%+v err=%v", list, err)
	}
	info, err := service.Info(context.Background(), "com.example.a")
	if err != nil || info.Name != "示例应用" || info.EnglishName != "Example" {
		t.Fatalf("info=%+v err=%v", info, err)
	}
}

func TestListUsesPackageManagerSystemClassification(t *testing.T) {
	executor := fakeExecutor{outputs: map[string]string{
		"pm list packages -f": "package:/cust/app/partner.apk=com.example.partner\npackage:/data/app/system.apk=com.example.system",
		"pm list packages -s": "package:com.example.system",
	}}
	list, err := New(executor, time.Second).List(context.Background(), true)
	if err != nil || len(list) != 2 || list[0].System || !list[1].System {
		t.Fatalf("list=%+v err=%v", list, err)
	}
}

func TestSessionResolvesAndStartsLauncherActivity(t *testing.T) {
	executor := fakeExecutor{outputs: map[string]string{
		"pm path com.example.a":                              "package:/data/app/a/base.apk",
		"dumpsys package com.example.a":                      "versionCode=42\n versionName=1.2.3",
		"cmd package resolve-activity --brief com.example.a": "com.example.a/.MainActivity",
		"am start -W -S -n com.example.a/.MainActivity":      "Status: ok",
	}}
	service := New(executor, time.Second)
	info, output, err := service.Session(context.Background(), "com.example.a")
	if err != nil {
		t.Fatal(err)
	}
	if info.MainActivity != ".MainActivity" || output != "Status: ok" {
		t.Fatalf("info=%+v output=%q", info, output)
	}
}

func TestLaunchUsesResolvedActivityInsteadOfMonkeyScript(t *testing.T) {
	executor := fakeExecutor{outputs: map[string]string{
		"pm path com.example.a":                              "package:/data/app/a/base.apk",
		"dumpsys package com.example.a":                      "versionCode=42\n versionName=1.2.3",
		"cmd package resolve-activity --brief com.example.a": "com.example.a/.MainActivity",
		"am start -W -n com.example.a/.MainActivity":         "Status: ok",
	}}
	service := New(executor, time.Second)
	output, err := service.Launch(context.Background(), "com.example.a")
	if err != nil || output != "Status: ok" {
		t.Fatalf("output=%q err=%v", output, err)
	}
}
