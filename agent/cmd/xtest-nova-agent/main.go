package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/zhoujun94511/xtest-nova/agent/internal/apps"
	"github.com/zhoujun94511/xtest-nova/agent/internal/artifacts"
	"github.com/zhoujun94511/xtest-nova/agent/internal/automation"
	"github.com/zhoujun94511/xtest-nova/agent/internal/autopopup"
	"github.com/zhoujun94511/xtest-nova/agent/internal/auxiliary"
	"github.com/zhoujun94511/xtest-nova/agent/internal/buildinfo"
	"github.com/zhoujun94511/xtest-nova/agent/internal/command"
	"github.com/zhoujun94511/xtest-nova/agent/internal/componenthealth"
	"github.com/zhoujun94511/xtest-nova/agent/internal/config"
	"github.com/zhoujun94511/xtest-nova/agent/internal/configstore"
	"github.com/zhoujun94511/xtest-nova/agent/internal/device"
	"github.com/zhoujun94511/xtest-nova/agent/internal/events"
	"github.com/zhoujun94511/xtest-nova/agent/internal/exploration"
	"github.com/zhoujun94511/xtest-nova/agent/internal/files"
	"github.com/zhoujun94511/xtest-nova/agent/internal/httpapi"
	"github.com/zhoujun94511/xtest-nova/agent/internal/minitouch"
	"github.com/zhoujun94511/xtest-nova/agent/internal/monitor"
	"github.com/zhoujun94511/xtest-nova/agent/internal/pkgmeta"
	"github.com/zhoujun94511/xtest-nova/agent/internal/platform"
	"github.com/zhoujun94511/xtest-nova/agent/internal/recordreplay"
	"github.com/zhoujun94511/xtest-nova/agent/internal/runner"
	"github.com/zhoujun94511/xtest-nova/agent/internal/runtimebootstrap"
	"github.com/zhoujun94511/xtest-nova/agent/internal/runtimebundle"
	"github.com/zhoujun94511/xtest-nova/agent/internal/scrcpy"
	"github.com/zhoujun94511/xtest-nova/agent/internal/screenrecord"
	novasystem "github.com/zhoujun94511/xtest-nova/agent/internal/system"
	"github.com/zhoujun94511/xtest-nova/agent/internal/tasks"
	"github.com/zhoujun94511/xtest-nova/agent/internal/touchreader"
)

func main() {
	closeDaemonLog, daemonLogErr := configureDaemonLog()
	if daemonLogErr != nil {
		log.Fatal(daemonLogErr)
	}
	defer closeDaemonLog()
	invocation, e := command.Parse(os.Args[1:])
	if e != nil {
		log.Fatal(e)
	}
	switch invocation.Kind {
	case command.Version:
		fmt.Println(buildinfo.Version)
		return
	case command.Popup:
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		if e = command.ManagePopup(ctx, invocation.PopupAction, platform.OSExecutor{}, os.Stdout); e != nil {
			log.Fatal(e)
		}
		return
	case command.Monkey:
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if e = command.ManageMonkey(ctx, invocation.MonkeyAction, command.DefaultAgentURL, nil, os.Stdout); e != nil {
			log.Fatal(e)
		}
		return
	case command.Server:
		if invocation.Stop {
			if e = stopDaemon(); e != nil {
				log.Fatal(e)
			}
			return
		}
		if invocation.Daemon {
			if e = startDaemon(invocation.ServerArgs); e != nil {
				log.Fatal(e)
			}
			return
		}
	}
	cfg, e := config.Parse(invocation.ServerArgs, os.Stderr)
	if e != nil {
		log.Fatal(e)
	}
	releasePID, e := claimPID()
	if e != nil {
		log.Fatal(e)
	}
	defer releasePID()
	executor := platform.OSExecutor{}
	bootstrapTargets := runtimebootstrap.Targets{
		Runner: cfg.RunnerClasspath, Companion: cfg.CompanionAPK,
		UIAutomatorHost: "/data/local/tmp/xtest-nova-uiautomator-host.apk",
		UIAutomatorTest: "/data/local/tmp/xtest-nova-uiautomator-test.apk",
	}
	bootstrap := runtimebootstrap.New(executor, runtimebundle.Embedded{}, bootstrapTargets)
	bootstrapContext, cancelBootstrap := context.WithTimeout(context.Background(), 2*time.Minute)
	bootstrapState, bootstrapErr := bootstrap.Ensure(bootstrapContext)
	cancelBootstrap()
	if bootstrapErr != nil {
		releasePID()
		log.Fatalf("complete runtime bootstrap failed: %v", bootstrapErr)
	}
	popupContext, cancelPopup := context.WithTimeout(context.Background(), 10*time.Second)
	popupStartAttempted, startupPopupErr := startPopupAtServerBoot(popupContext, cfg.NoPopup, executor, os.Stdout)
	cancelPopup()
	if startupPopupErr != nil {
		log.Printf("Companion overlay did not start automatically: %v", startupPopupErr)
	}
	d := device.New(executor, cfg.CommandTimeout)
	packageMetadata := pkgmeta.New(executor, cfg.CommandTimeout)
	applications := apps.New(executor, cfg.CommandTimeout, packageMetadata)
	fileSystem := files.New("/data/local/tmp", "/sdcard", "/storage/emulated/0")
	automator := automation.New(executor, cfg.CommandTimeout, cfg.LegacyUiAutomator)
	if e = automator.ConfigureNova(cfg.HierarchyProvider, cfg.UiAutomatorAddress); e != nil {
		log.Fatal(e)
	}
	if cfg.HierarchyProvider == "nova" {
		if startErr := automator.Start(); startErr != nil {
			log.Printf("Nova hierarchy provider unavailable at startup; system fallback remains active: %v", startErr)
		}
	}
	store, e := configstore.New(cfg.ConfigPath)
	if e != nil {
		log.Fatalf("load config store: %v", e)
	}
	popup := autopopup.New(automator, executor, store.Get)
	systemService := novasystem.New(executor, cfg.CommandTimeout)
	primaryListener, e := net.Listen("tcp", cfg.ListenAddress)
	if e != nil {
		releasePID()
		log.Fatalf("bind Agent listener %s: %v", cfg.ListenAddress, e)
	}
	defer func() {
		if closeErr := primaryListener.Close(); closeErr != nil && !errors.Is(closeErr, net.ErrClosed) {
			log.Printf("close Agent listener: %v", closeErr)
		}
	}()
	probeContext, cancelProbe := context.WithTimeout(context.Background(), 2*time.Second)
	companionListener, companionReused, companionErr := auxiliary.ListenOrReuse(probeContext, cfg.CompanionAddress, auxiliary.HTTPProbe{Path: "/health", Service: "xtest-nova-companion", Version: buildinfo.Version})
	cancelProbe()
	if companionErr != nil {
		log.Printf("Companion degraded on %s: %v", cfg.CompanionAddress, companionErr)
	}
	if companionReused {
		log.Printf("reusing compatible Companion service on %s", cfg.CompanionAddress)
	}
	if companionListener != nil {
		defer func() {
			if closeErr := companionListener.Close(); closeErr != nil && !errors.Is(closeErr, net.ErrClosed) {
				log.Printf("close Companion listener: %v", closeErr)
			}
		}()
	}
	monitorService := monitor.New(systemService, cfg.MonitorAddress)
	if e = monitorService.Start(); e != nil {
		if probeErr := monitor.Probe(cfg.MonitorAddress, 2*time.Second); probeErr == nil {
			monitorService.UseExternal()
			log.Printf("reusing compatible Monitor service on %s", cfg.MonitorAddress)
		} else {
			log.Printf("Monitor degraded on %s: bind: %v; compatibility probe: %v", cfg.MonitorAddress, e, probeErr)
		}
	}
	defer func() {
		if closeErr := monitorService.Close(); closeErr != nil && !errors.Is(closeErr, net.ErrClosed) {
			log.Printf("close Monitor listener: %v", closeErr)
		}
	}()
	runs := runner.NewWithArtifacts(runner.AppProcessFactory{Classpath: cfg.RunnerClasspath, StopFile: cfg.StopFile},
		cfg.StopFile, cfg.LogFile, artifacts.Root, automator, packageMetadata)
	touchManager := minitouch.New("/data/local/tmp/minitouch", "/data/local/tmp/xtest-nova-minitouch.log", "@minitouch")
	recorder := screenrecord.New(artifacts.Root)
	scrcpyManager := scrcpy.New()
	explorer := exploration.New(automator, d, executor, scrcpyManager)
	recordReplay := recordreplay.New(artifacts.Root, touchreader.OSCapture{}, d, executor, scrcpyManager, automator)
	taskManager := tasks.New()
	var lanAuth []httpapi.LANAuthConfig
	if cfg.AllowLAN {
		lanAuth = append(lanAuth, httpapi.LANAuthConfig{Token: cfg.APIToken()})
	}
	api := httpapi.New(d, runs, store, applications, fileSystem, automator, systemService, taskManager, touchManager, popup, events.New(), monitorService, recorder, scrcpyManager, explorer, recordReplay, cfg.CompanionAPK, cfg.UnsafeLegacyAPI, lanAuth...)
	for index := range lanAuth {
		clear(lanAuth[index].Token)
	}
	cfg.ClearAPIToken()
	api.SetLegacyUiAutomator(cfg.LegacyUiAutomator)
	companionReady := companionListener != nil || companionReused
	companionDetail := ""
	if companionReused {
		companionDetail = "compatible external listener reused"
	} else if companionErr != nil {
		companionDetail = companionErr.Error()
	}
	companionStatus := componenthealth.New("companion", true, companionReady, companionReady, buildinfo.Version, companionDetail)
	if !companionReady {
		companionStatus = componenthealth.Degraded("companion", true, buildinfo.Version, companionDetail)
	}
	api.SetAuxiliaryStatus(companionStatus)
	if !popupStartAttempted {
		api.SetAuxiliaryStatus(componenthealth.New("popupOverlayStartup", true, false, false, buildinfo.Version, "disabled by --no-popup"))
	} else if startupPopupErr != nil {
		api.SetAuxiliaryStatus(componenthealth.Degraded("popupOverlayStartup", true, buildinfo.Version, startupPopupErr.Error()))
	} else {
		api.SetAuxiliaryStatus(componenthealth.New("popupOverlayStartup", true, true, true, buildinfo.Version, "started automatically with server"))
	}
	api.SetAuxiliaryStatus(componenthealth.New("runtimeBootstrap", true, bootstrapState.Ready, false, buildinfo.Version, bootstrapState.Error))
	for _, component := range bootstrapState.Components {
		api.SetAuxiliaryStatus(componenthealth.New(component.Name+"Payload", true, component.Ready, false, "", component.Action))
	}
	stopRequested := make(chan struct{})
	var stopOnce sync.Once
	requestStop := func() { stopOnce.Do(func() { close(stopRequested) }) }
	api.SetShutdown(requestStop)
	servers := []*http.Server{{Addr: cfg.ListenAddress, Handler: api.Primary(), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 64 << 10}}
	listeners := []net.Listener{primaryListener}
	if companionListener != nil {
		servers = append(servers, &http.Server{Addr: cfg.CompanionAddress, Handler: api.Companion(), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 64 << 10})
		listeners = append(listeners, companionListener)
	}
	serverErrors := make(chan error, len(servers))
	for index, s := range servers {
		listener := listeners[index]
		go func(server *http.Server) {
			log.Printf("listening on %s", listener.Addr())
			if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
				serverErrors <- fmt.Errorf("server %s failed: %w", listener.Addr(), err)
			}
		}(s)
	}
	if os.Getenv("XTEST_NOVA_DAEMON_LOG") == "" {
		if err := writeWebAccessInstructions(os.Stdout, cfg); err != nil {
			log.Printf("write Web access instructions: %v", err)
		}
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-signals:
	case <-stopRequested:
	case serverErr := <-serverErrors:
		log.Print(serverErr)
	}
	signal.Stop(signals)
	api.BeginShutdown()
	// Stop accepting requests and drain ordinary in-flight handlers before
	// taking the resource snapshot below. Otherwise, a late mutating request can
	// recreate a process or task after its manager has already been stopped.
	for _, s := range servers {
		withStopContext(5*time.Second, func(ctx context.Context) {
			if e := s.Shutdown(ctx); e != nil {
				_, _ = fmt.Fprintln(os.Stderr, e)
				_ = s.Close()
			}
		})
	}
	_, _ = recorder.Stop()
	withStopContext(5*time.Second, func(ctx context.Context) { _, _ = explorer.Stop(ctx) })
	withStopContext(5*time.Second, func(ctx context.Context) { _, _ = recordReplay.StopRecording(ctx) })
	withStopContext(5*time.Second, func(ctx context.Context) { _, _ = recordReplay.StopReplay(ctx) })
	withStopContext(5*time.Second, api.StopPerformance)
	withStopContext(5*time.Second, func(ctx context.Context) { _ = api.StopSoak(ctx) })
	withStopContext(5*time.Second, func(ctx context.Context) { _ = api.StopBackground(ctx) })
	scrcpyManager.Close()
	_, _ = popup.Stop()
	_, _ = touchManager.Stop()
	_, _ = runs.Stop()
	withStopContext(runner.FinalizationTimeout, func(ctx context.Context) {
		if _, err := runs.WaitFinalized(ctx); err != nil {
			log.Printf("Runner artifact finalization did not complete before shutdown: %v", err)
		}
	})
	withStopContext(5*time.Second, func(ctx context.Context) { _ = automator.Stop(ctx) })
	withStopContext(5*time.Second, func(ctx context.Context) { _ = taskManager.CancelAll(ctx) })
}

func startPopupAtServerBoot(ctx context.Context, noPopup bool, executor platform.Executor, output io.Writer) (bool, error) {
	if noPopup {
		return false, nil
	}
	return true, command.ManagePopup(ctx, "start", executor, output)
}

func withStopContext(timeout time.Duration, action func(context.Context)) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	action(ctx)
}
