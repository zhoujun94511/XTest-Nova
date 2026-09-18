package pkgmeta

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type fakeExecutor struct {
	output []byte
	err    error
	calls  int
}

func (f *fakeExecutor) Run(context.Context, string, ...string) (string, error) { return "", f.err }
func (f *fakeExecutor) RunBytes(_ context.Context, name string, args ...string) ([]byte, error) {
	f.calls++
	if name != "content" || len(args) != 3 || args[0] != "read" || args[1] != "--uri" {
		return nil, errors.New("unexpected metadata command")
	}
	return f.output, f.err
}

func TestActivitiesNormalizesAndSortsPackageManagerOutput(t *testing.T) {
	executor := &fakeExecutor{output: []byte("com.example.app.SettingsActivity\ncom.vendor.SharedActivity\ncom.example.app.MainActivity\ncom.example.app.MainActivity\n")}
	got, err := New(executor, 0).Activities(context.Background(), "com.example.app")
	want := []string{"com.example.app/.MainActivity", "com.example.app/.SettingsActivity", "com.example.app/com.vendor.SharedActivity"}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("activities=%#v err=%v, want %#v", got, err, want)
	}
}

func TestActivitiesRejectsUnexpectedProviderOutput(t *testing.T) {
	service := New(&fakeExecutor{output: []byte("not/a/class\n")}, 0)
	if _, err := service.Activities(context.Background(), "com.example.app"); err == nil {
		t.Fatal("invalid provider output accepted")
	}
}

func TestIconValidatesJPEGAndCommandFailure(t *testing.T) {
	executor := &fakeExecutor{output: []byte{0xff, 0xd8, 0xff, 0xd9}}
	if value, err := New(executor, 0).Icon(context.Background(), "com.example.app"); err != nil || len(value) != 4 {
		t.Fatalf("icon bytes=%d err=%v", len(value), err)
	}
	executor.err = errors.New("provider unavailable")
	if _, err := New(executor, 0).Icon(context.Background(), "com.example.app"); err == nil {
		t.Fatal("provider failure was ignored")
	}
}

func TestRejectsInvalidPackageBeforeCommand(t *testing.T) {
	executor := &fakeExecutor{}
	if _, err := New(executor, 0).Activities(context.Background(), "../settings"); err == nil || executor.calls != 0 {
		t.Fatalf("err=%v calls=%d", err, executor.calls)
	}
}

func TestLabelsAndCatalogDecodeLocalizedNames(t *testing.T) {
	labels, err := New(&fakeExecutor{output: []byte(`{"zh":" 相册 ","en":"Gallery"}`)}, 0).Labels(context.Background(), "com.example.gallery")
	if err != nil || labels.Chinese != "相册" || labels.English != "Gallery" {
		t.Fatalf("labels=%+v err=%v", labels, err)
	}
	catalog, err := New(&fakeExecutor{output: []byte(`[{"packageName":"com.example.gallery","zh":"相册","en":"Gallery"},{"packageName":"../bad","zh":"bad"}]`)}, 0).Catalog(context.Background())
	if err != nil || len(catalog) != 1 || catalog["com.example.gallery"].Chinese != "相册" {
		t.Fatalf("catalog=%+v err=%v", catalog, err)
	}
}
