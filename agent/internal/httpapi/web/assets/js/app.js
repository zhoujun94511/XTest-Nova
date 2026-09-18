(() => {
  'use strict';
  const C = window.NovaCore;
  const R = window.NovaRenderers;
  const P = window.NovaPackageSelector;
  const Remote = window.NovaRemoteControl;
  const PerformanceView = window.NovaPerformanceView;
  const {q, qa, api, jsonBody, required, numberValue, checked, fileURL, fileInfoURL, archiveURL, toast, setButtonBusy} = C;

  /**
   * @typedef {Object} SessionIdentity
   * @property {string} [sessionId]
   * @property {string} [ownerToken]
   */
  /**
   * @typedef {Object} OwnedSessionState
   * @property {SessionIdentity} [identity]
   */
  /**
   * @typedef {Object} StartupReport
   * @property {string} [verdict]
   */
  /**
   * @typedef {Object} LANAuthStatus
   * @property {boolean} [enabled]
   * @property {boolean} [authenticated]
   */

  const viewMeta = {
    overview: ['工作台', '运行概览', '掌握设备、运行组件和测试任务的实时状态。'],
    apps: ['目标管理', '应用管理', '查找测试目标，查看包信息并快速启动。'],
    automation: ['测试执行', '自动化测试', '随机遍历、智能探索和录制回放。'],
    performance: ['指标采集', '性能采集', '持续记录目标应用的运行性能。'],
    remote: ['设备交互', '屏幕查看与远程控制', '实时查看设备画面，并通过鼠标、触屏和键盘操作。'],
    artifacts: ['结果中心', '测试产物', '检索运行结果并下载诊断文件。'],
    diagnostics: ['运行保障', '运行诊断', '检查 Agent 资源、组件和日志。']
  };

  const model = {
    packages: [],
    filteredPackages: [],
    selectedPackage: '',
    cases: [],
    selectedCaseID: '',
    fileDirectory: '',
    selectedFile: '',
    selectedFileSize: 0,
    fileTreeLoaded: false,
    activeView: 'overview',
    polling: false,
    pollTimer: 0,
    sessions: null,
    monkeyState: null,
    explorationState: null,
    recordState: null,
    replayState: null,
    performanceState: null
  };
  let lanMode = false;
  let initialized = false;

  const fetchResource = async (path, options = {}) => {
    const response = await fetch(path, {credentials: 'same-origin', ...options});
    if (response.status === 401) window.dispatchEvent(new CustomEvent('nova-auth-required'));
    return response;
  };

  /** @param {OwnedSessionState|null|undefined} state */
  const ownerHeaders = state => {
    const identity = state?.identity || {};
    return {'X-XTest-Session-Id': identity.sessionId || '', 'X-XTest-Owner-Token': identity.ownerToken || ''};
  };

  const packageName = () => {
    const value = P.getValue();
    if (!value) {
      const selector = model.activeView === 'performance' ? '#performance-package' : '#automation-package';
      (q(selector) || q('#app-search')).focus();
      throw new Error('请先选择目标应用');
    }
    return value;
  };

  const selectPackage = value => {
	model.selectedPackage = String(value || '').trim();
	const selected = model.packages.find(item => item.packageName === model.selectedPackage);
	q('#selected-app-name').textContent = selected?.name || model.selectedPackage || '尚未选择';
    P.setValue(model.selectedPackage);
	q('#selected-package').textContent = model.selectedPackage || '请选择目标应用';
    q('#perf-target').textContent = model.selectedPackage || '尚未选择应用';
    q('#artifact-package').value = model.selectedPackage;
    filterApps();
    void loadAppDetails(true);
  };

  const filterApps = () => {
    const needle = (q('#app-search')?.value || '').trim().toLowerCase();
    model.filteredPackages = model.packages.filter(item => {
      const text = typeof item === 'string' ? item : `${item.name || item.label || ''} ${item.packageName || ''}`;
      return text.toLowerCase().includes(needle);
    });
    R.renderApps(model.filteredPackages, model.selectedPackage);
  };

  const setHealth = (ok, label, version = '') => {
    q('#health-label').textContent = label;
    q('#version').textContent = version || 'Agent 不可用';
    q('#health').lastElementChild.textContent = label;
    [q('#health-dot'), q('#health .status-dot')].forEach(dot => { dot.className = `status-dot ${ok ? 'ok' : 'bad'}`; });
  };

  const openView = name => {
    if (!viewMeta[name]) name = 'overview';
    model.activeView = name;
    qa('.nav-item').forEach(item => {
      const active = item.dataset.view === name;
      item.classList.toggle('active', active);
      if (active) item.setAttribute('aria-current', 'page'); else item.removeAttribute('aria-current');
    });
    qa('.view').forEach(view => view.classList.toggle('active', view.id === name));
    const [kicker, title, description] = viewMeta[name];
    q('#view-kicker').textContent = kicker;
    q('#view-title').textContent = title;
    q('#view-description').textContent = description;
    if (location.hash !== `#${name}`) history.replaceState(null, '', `#${name}`);
    q('#workspace').focus({preventScroll: true});
    if (name === 'artifacts' && !model.fileTreeLoaded) void loadFileDirectory(q('#file-root').value).catch(error => toast(error.message, 'error'));
    if (name === 'performance') void refreshPerformance(true).catch(error => toast(error.message, 'error'));
    if (name === 'remote') Remote.activate(); else Remote.deactivate();
    if (model.sessions) schedulePoll(0);
  };

  async function loadDashboard(silent = false) {
    const results = await Promise.allSettled([
      api('/v1/health'), api('/v1/device'), api('/v1/diagnostics/components'), api('/v1/sessions/current')
    ]);
    if (results[0].status === 'rejected') {
      setHealth(false, '连接失败');
      if (!silent) toast(results[0].reason.message, 'error');
      return;
    }
    const health = results[0].value;
    setHealth(health.status === 'ok', health.status === 'ok' ? '服务正常' : '服务降级', health.version);
    if (results[1].status === 'fulfilled') R.renderDevice(results[1].value);
    if (results[2].status === 'fulfilled') {
      R.renderReadiness(results[2].value);
      R.renderComponents(results[2].value);
    }
    if (results[3].status === 'fulfilled') {
      model.sessions = results[3].value.sessions || {};
      renderSessionBanner(model.sessions);
    }
  }

  const renderSessionBanner = sessions => {
    const labels = {runner: '随机遍历', exploration: '智能探索', recording: '录制', replay: '回放', screenRecord: '屏幕录制', performance: '性能采集'};
    const active = Object.entries(sessions).filter(([, state]) => typeof state === 'boolean' ? state : state && (state.running || state.stopping || state.finalizing)).map(([name]) => labels[name] || name);
    const banner = q('#active-session');
    banner.hidden = active.length === 0;
    if (active.length) banner.textContent = `正在运行：${active.join('、')}。互斥任务启动前请先停止当前任务。`;
  };

  const loadApps = async (silent = false) => {
	const scope = q('#app-scope').value;
	model.packages = await api(`/packages?scope=${encodeURIComponent(scope)}`);
    P.setPackages(model.packages);
    P.setValue(model.selectedPackage);
    q('#artifact-package').value = model.selectedPackage;
    filterApps();
    if (!silent) toast(`已读取 ${model.packages.length} 个应用`);
  };

  const refreshMonkey = async (silent = false) => {
    const state = await api('/v1/monkey/runs/current', {uiSilent: silent});
    R.setPill('#monkey-state-pill', state);
    R.renderData('#monkey-output', state, ['running', 'events', 'crashCount', 'anrCount', 'nativeCrashCount', 'diagnosticsComplete']);
    model.monkeyState = state;
    return state;
  };
  const refreshExploration = async (silent = false) => {
    const state = await api('/v1/exploration/sessions/current', {uiSilent: silent});
    R.setPill('#exploration-state-pill', state);
    R.renderData('#explore-output', state, ['running', 'steps', 'discoveredStates', 'discoveredEdges', 'effectiveActions', 'ineffectiveActions', 'externalActions', 'staleActions', 'activityCovered', 'cycleDetections']);
    model.explorationState = state;
    return state;
  };
  const refreshPerformance = async (silent = false) => {
    const state = await api('/v1/performance/sessions/current', {uiSilent: silent});
    model.performanceState = state;
    R.setPill('#perf-state-pill', state);
    PerformanceView.render(state);
    if (!state.running && state.summaryPath) {
      try {
        const response = await fetchResource(fileURL(state.summaryPath), {headers: {'Accept': 'application/json'}});
        if (response.ok) PerformanceView.renderSummary(JSON.parse(await response.text()));
      } catch (_) { /* Raw summary remains available through the artifact link. */ }
    }
    return state;
  };

  const refreshDiagnostics = async (silent = false) => {
    const [runtime, components] = await Promise.all([
      api('/v1/diagnostics/runtime', {uiSilent: silent}),
      api('/v1/diagnostics/components', {uiSilent: silent})
    ]);
    R.renderRuntime(runtime);
    R.renderComponents(components);
    R.renderReadiness(components);
    renderSessionBanner(runtime.sessions || {});
    if (!silent) toast('诊断和组件状态已刷新');
  };

  const renderSoakState = state => {
    R.setPill('#soak-state-pill', state);
    R.renderData('#soak-output', state);
    return state;
  };
  const refreshRecordReplay = async (type, silent = false) => {
    const path = type === 'replay' ? '/v1/replays/current' : '/v1/recordings/current';
    const state = await api(path, {uiSilent: silent});
    R.setPill('#record-state-pill', state);
    R.renderData('#record-state-output', state, ['running', 'name', 'actions', 'completedActions', 'path', 'error']);
    model[`${type}State`] = state;
    return state;
  };

  const loadCases = async () => {
    const suffix = model.selectedPackage ? `?package=${encodeURIComponent(model.selectedPackage)}` : '';
    const payload = await api(`/v1/recordings${suffix}`);
    model.cases = payload.cases || [];
    if (!model.cases.some(item => item.id === model.selectedCaseID)) model.selectedCaseID = model.cases[0]?.id || '';
    R.renderCases(model.cases, model.selectedCaseID);
  };

  const loadAppDetails = async silent => {
    const host = q('#app-output');
    const icon = q('#selected-app-icon');
    const fallback = q('#selected-app-fallback');
    icon.hidden = true;
    icon.removeAttribute('src');
    fallback.textContent = model.selectedPackage.slice(0, 1).toUpperCase() || 'A';
    if (!model.selectedPackage) {
      host.className = 'structured-output empty-state';
      host.textContent = '请从应用列表选择目标应用。';
      return;
    }
    try {
	  const payload = await api(`/packages/${encodeURIComponent(model.selectedPackage)}/info`, {uiSilent: Boolean(silent)});
	  const details = payload?.data || payload;
	  q('#selected-app-name').textContent = details.name || model.selectedPackage;
      host.className = 'structured-output';
      R.renderAppDetails(payload);
      icon.onload = () => { icon.hidden = false; };
      icon.onerror = () => { icon.hidden = true; };
      icon.src = `/packages/${encodeURIComponent(model.selectedPackage)}/icon`;
    } catch (error) {
      if (!silent) throw error;
    }
  };

  const clearFileSelection = () => {
    model.selectedFile = '';
    model.selectedFileSize = 0;
    q('#file-selection').hidden = true;
    q('#file-preview').hidden = true;
    q('#file-output').hidden = true;
    q('#file-output').replaceChildren();
  };

  const loadFileDirectory = async path => {
    const entry = await api(fileInfoURL(path));
    if (!entry.isDirectory) throw new Error('所选路径不是文件夹');
    model.fileDirectory = entry.path;
    model.fileTreeLoaded = true;
    clearFileSelection();
    R.renderFileTree(entry);
  };

  const selectFile = (path, size) => {
    model.selectedFile = path;
    model.selectedFileSize = Number(size) || 0;
    q('#file-selected-name').textContent = path.split('/').pop() || path;
    q('#file-selected-path').textContent = path;
    q('#file-selection').hidden = false;
    q('#file-preview').hidden = true;
    q('#file-output').hidden = true;
  };

  const actions = {
    'refresh-dashboard': () => loadDashboard().then(() => toast('控制台状态已刷新')),
    screenshot: async () => {
      const response = await fetchResource('/screenshot/0');
      if (!response.ok) throw new Error(`截图失败 (${response.status})`);
      const image = q('#shot');
      if (image.dataset.objectUrl) URL.revokeObjectURL(image.dataset.objectUrl);
      image.dataset.objectUrl = URL.createObjectURL(await response.blob());
      image.src = image.dataset.objectUrl;
      q('#screenshot-panel').hidden = false;
      q('#screenshot-panel').scrollIntoView({behavior: 'smooth', block: 'center'});
      toast('已获取设备截图');
    },
    'close-screenshot': () => { q('#screenshot-panel').hidden = true; },
    'popup-start': () => api('/popupBoxAssistant', {method: 'POST'}).then(() => toast('悬浮窗启动命令已发送')),
    'popup-stop': () => api('/popupBoxAssistant', {method: 'DELETE'}).then(() => toast('悬浮窗已停止')),
    'apps-refresh': loadApps,
    'app-launch': () => api(`/v1/apps/${encodeURIComponent(packageName())}/launch`, {method: 'POST'}).then(() => toast('目标应用已启动')),
    'app-info': () => loadAppDetails(false),
    'app-uninstall': async () => {
      const name = packageName();
      const protectedPackages = new Set(['com.openatx.xtest.popup', 'com.openatx.xtest.nova.uiautomator', 'com.openatx.xtest.nova.uiautomator.test']);
      if (protectedPackages.has(name)) throw new Error('该应用属于 XTest Nova 运行组件，不能从控制台卸载');
      if (!window.confirm(`确定卸载 ${name}？应用数据也会被删除。`)) return;
      await api(`/v1/apps/${encodeURIComponent(name)}`, {method: 'DELETE'});
      selectPackage('');
      await loadApps(true);
      toast('应用已卸载');
    },
    'monkey-start': async () => {
      await api('/v1/monkey/runs/current', jsonBody({package: packageName(), durationSeconds: numberValue('#monkey-duration'), throttleMillis: numberValue('#monkey-throttle'), renderFallbackMode: q('#monkey-render-fallback').value}));
      await refreshMonkey(); toast('随机遍历已开始');
    },
    'monkey-refresh': refreshMonkey,
    'monkey-stop': async () => { if (!model.monkeyState) await refreshMonkey(true); await api('/v1/monkey/runs/current', {method: 'DELETE', headers: ownerHeaders(model.monkeyState)}); await refreshMonkey(); toast('随机遍历已停止，产物正在收尾'); },
    'explore-preview': async () => R.renderData('#explore-output', await api('/v1/exploration/preview', jsonBody({package: packageName()}))),
    'explore-start': async () => {
      await api('/v1/exploration/sessions', jsonBody({package: packageName(), maxSteps: numberValue('#explore-steps'), intervalMillis: numberValue('#explore-interval'), execute: checked('#explore-execute'), enableScroll: checked('#explore-scroll'), enableBacktrack: checked('#explore-backtrack'), recoverPopups: checked('#explore-recover'), maxBacktracks: numberValue('#explore-backtracks')}));
      await refreshExploration(); toast('智能探索已开始');
    },
    'explore-refresh': refreshExploration,
    'explore-stop': async () => { if (!model.explorationState) await refreshExploration(true); await api('/v1/exploration/sessions/current', {method: 'DELETE', headers: ownerHeaders(model.explorationState)}); await refreshExploration(); toast('智能探索已停止'); },
    'record-start': async () => {
      const name = required('#record-name', '请输入用例名称');
      await api('/v1/recordings', jsonBody({package: packageName(), name, task: q('#record-task').value.trim()}));
      await refreshRecordReplay('record'); toast('录制已开始，请在设备上完成操作');
    },
    'record-stop': async () => { if (!model.recordState) await refreshRecordReplay('record', true); await api('/v1/recordings/current', {method: 'DELETE', headers: ownerHeaders(model.recordState)}); await refreshRecordReplay('record'); await loadCases(); toast('录制已停止并保存'); },
    'record-state': () => refreshRecordReplay('record'),
    'record-list': loadCases,
    'replay-start': async () => {
      if (!model.selectedCaseID) throw new Error('请先选择一个已保存用例');
      const recordedCase = await api(`/v1/recordings/cases/${encodeURIComponent(model.selectedCaseID)}`);
      await api('/v1/replays', jsonBody({execute: true, speed: Number(q('#replay-speed').value), caseFingerprint: recordedCase.integrity, case: recordedCase}));
      await refreshRecordReplay('replay'); toast('用例回放已开始');
    },
    'replay-stop': async () => { if (!model.replayState) await refreshRecordReplay('replay', true); await api('/v1/replays/current', {method: 'DELETE', headers: ownerHeaders(model.replayState)}); await refreshRecordReplay('replay'); toast('用例回放已停止'); },
    'replay-state': () => refreshRecordReplay('replay'),
    'perf-start': async () => { PerformanceView.reset(); await api('/v1/performance/sessions', jsonBody({package: packageName(), intervalSeconds: Number(q('#perf-interval').value), durationSeconds: Number(q('#perf-duration').value)})); await refreshPerformance(); toast('性能采集已开始'); },
    'perf-refresh': refreshPerformance,
    'perf-stop': async () => {
      if (!model.performanceState) await refreshPerformance(true);
      await api('/v1/performance/sessions/current', {method: 'DELETE', headers: ownerHeaders(model.performanceState)});
      await refreshPerformance(); toast('性能数据已保存');
    },
    'startup-measure': async () => {
      /** @type {StartupReport} */
      const report = await api('/v1/performance/startup', jsonBody({package: packageName(), mode: q('#startup-mode').value, runs: numberValue('#startup-runs'), cooldownMillis: numberValue('#startup-cooldown'), baselineP95Millis: numberValue('#startup-baseline'), maxRegressionPercent: numberValue('#startup-regression')}));
      R.renderData('#startup-output', report, ['mode', 'verdict', 'reason', 'statistics', 'path']);
      toast(report.verdict === 'failed' ? '启动性能超过基线' : '启动性能测量完成', report.verdict === 'failed' ? 'error' : 'success');
    },
    'artifacts-refresh': async () => {
      const params = new URLSearchParams({limit: q('#artifact-limit').value});
      const packageFilter = q('#artifact-package').value.trim();
      const kind = q('#artifact-kind').value;
      if (packageFilter) params.set('package', packageFilter);
      if (kind) params.set('kind', kind);
      R.renderArtifacts(await api(`/v1/diagnostics/artifacts?${params}`));
    },
    'file-refresh': () => loadFileDirectory(model.fileDirectory || q('#file-root').value),
    'file-export-directory': () => window.open(archiveURL(model.fileDirectory || q('#file-root').value), '_blank', 'noopener'),
    'file-up': () => {
      const root = q('#file-root').value;
      const current = model.fileDirectory || root;
      if (current === root) { toast('已经位于当前浏览位置的顶层'); return; }
      const parent = current.slice(0, current.lastIndexOf('/')) || root;
      return loadFileDirectory(parent.startsWith(root) ? parent : root);
    },
    'file-info': async () => {
      if (!model.selectedFile) throw new Error('请先从文件列表中选择文件');
      q('#file-output').hidden = false;
      R.renderData('#file-output', await api(fileInfoURL(model.selectedFile)));
    },
    'file-preview': async () => {
      if (!model.selectedFile) throw new Error('请先从文件列表中选择文件');
      const extension = (model.selectedFile.split('.').pop() || '').toLowerCase();
      const url = fileURL(model.selectedFile);
      const name = model.selectedFile.split('/').pop();
      if (['png', 'jpg', 'jpeg', 'gif', 'webp', 'bmp'].includes(extension)) { R.renderFilePreview({kind: 'image', url, name}); return; }
      if (['mp3', 'wav', 'm4a', 'aac', 'ogg', 'flac'].includes(extension)) { R.renderFilePreview({kind: 'audio', url, name}); return; }
      if (['mp4', 'webm', 'mov', 'm4v'].includes(extension)) { R.renderFilePreview({kind: 'video', url, name}); return; }
      if (extension === 'pdf') { R.renderFilePreview({kind: 'pdf', url, name}); return; }
      if (['txt', 'log', 'json', 'jsonl', 'xml', 'csv', 'md', 'yaml', 'yml'].includes(extension)) {
        if (model.selectedFileSize > 2 * 1024 * 1024) { R.renderFilePreview({message: '文本文件超过 2 MB，为避免占用过多浏览器内存，请下载后查看。'}); return; }
        const response = await fetchResource(url);
        if (!response.ok) throw new Error(`预览读取失败 (${response.status})`);
        R.renderFilePreview({kind: 'text', text: await response.text(), name});
        return;
      }
      R.renderFilePreview({message: '暂不支持预览此文件格式，可直接下载。'});
    },
    'file-download': () => {
      if (!model.selectedFile) throw new Error('请先从文件列表中选择文件');
      window.open(fileURL(model.selectedFile), '_blank', 'noopener');
    },
    'diagnostics-refresh': () => refreshDiagnostics(),
    'components-refresh': async () => { const payload = await api('/v1/diagnostics/components'); R.renderComponents(payload); R.renderReadiness(payload); },
    'logs-clear': () => { q('#logs-output').textContent = '日志视图已清空，设备日志未被删除。'; },
    'logs-refresh': async () => {
      const params = new URLSearchParams({lines: q('#log-lines').value});
      const contains = q('#log-contains').value.trim();
      if (contains) params.set('contains', contains);
      const payload = await api(`/v1/diagnostics/logs/${encodeURIComponent(q('#log-name').value)}?${params}`);
      q('#logs-output').textContent = Array.isArray(payload.lines) ? payload.lines.join('\n') : payload.content || payload.text || JSON.stringify(payload, null, 2);
    },
    'soak-start': async () => { renderSoakState(await api('/v1/diagnostics/soak', jsonBody({durationSeconds: numberValue('#soak-duration'), intervalSeconds: numberValue('#soak-interval')}))); toast('稳定性采样已开始'); },
    'soak-state': async () => renderSoakState(await api('/v1/diagnostics/soak/current')),
    'soak-stop': async () => { renderSoakState(await api('/v1/diagnostics/soak/current', {method: 'DELETE'})); toast('稳定性采样已停止'); }
  };

  const sessionMutationActions = new Set([
    'monkey-start', 'monkey-stop', 'explore-start', 'explore-stop', 'record-start', 'record-stop',
    'replay-start', 'replay-stop', 'perf-start', 'perf-stop'
  ]);

  document.addEventListener('click', async event => {
    const nav = event.target.closest('[data-view]');
    if (nav) { openView(nav.dataset.view); return; }
    const app = event.target.closest('[data-package]');
    if (app) { selectPackage(app.dataset.package); return; }
    const file = event.target.closest('[data-file-path]');
    if (file) {
      try {
        if (file.dataset.fileDirectory === 'true') await loadFileDirectory(file.dataset.filePath);
        else selectFile(file.dataset.filePath, file.dataset.fileSize);
      } catch (error) { toast(error.message || String(error), 'error'); }
      return;
    }
    const savedCase = event.target.closest('[data-case-id]');
    if (savedCase) { model.selectedCaseID = savedCase.dataset.caseId; R.renderCases(model.cases, model.selectedCaseID); return; }
    const button = event.target.closest('[data-action]');
    if (!button || !actions[button.dataset.action]) return;
    setButtonBusy(button, true);
    try {
      await actions[button.dataset.action]();
      if (sessionMutationActions.has(button.dataset.action)) schedulePoll(0);
    }
    catch (error) { toast(error.message || String(error), 'error'); }
    finally { setButtonBusy(button, false); }
  });

  P.configure(selectPackage);
  Remote.configure({toast});
  q('#app-search').addEventListener('input', filterApps);
  q('#app-scope').addEventListener('change', () => loadApps().catch(error => toast(error.message, 'error')));
  q('#file-root').addEventListener('change', event => loadFileDirectory(event.target.value).catch(error => toast(error.message, 'error')));
  window.addEventListener('hashchange', () => openView(location.hash.slice(1)));
  window.addEventListener('pagehide', () => Remote.deactivate());
  document.addEventListener('visibilitychange', () => {
    if (document.hidden) {
      if (model.pollTimer) window.clearTimeout(model.pollTimer);
      model.pollTimer = 0;
    } else {
      schedulePoll(0);
    }
  });

  async function poll() {
    if (document.hidden) return;
    if (model.polling) { schedulePoll(1000); return; }
    model.polling = true;
    let nextDelay = 10000;
    try {
      const summary = await api('/v1/sessions/current', {uiSilent: true});
      const sessions = summary.sessions || {};
      const previous = model.sessions || {};
      const busy = state => Boolean(state && (state.running || state.stopping || state.finalizing));
      const wasOrIsBusy = name => busy(sessions[name]) || busy(previous[name]);
      renderSessionBanner(sessions);
      R.setPill('#monkey-state-pill', sessions.runner || {});
      R.setPill('#exploration-state-pill', sessions.exploration || {});
      R.setPill('#perf-state-pill', sessions.performance || {});
      R.setPill('#record-state-pill', busy(sessions.replay) ? sessions.replay : sessions.recording || {});

      const details = [];
      if (model.activeView === 'automation') {
        if (wasOrIsBusy('runner')) details.push(refreshMonkey(true));
        if (wasOrIsBusy('exploration')) details.push(refreshExploration(true));
        if (wasOrIsBusy('replay')) details.push(refreshRecordReplay('replay', true));
        else if (wasOrIsBusy('recording')) details.push(refreshRecordReplay('record', true));
      } else if (model.activeView === 'performance' && wasOrIsBusy('performance')) {
        details.push(refreshPerformance(true));
      } else if (model.activeView === 'diagnostics') {
        details.push(api('/v1/diagnostics/runtime', {uiSilent: true}).then(R.renderRuntime));
      }
      model.sessions = sessions;
      if (details.length) await Promise.allSettled(details);
      nextDelay = summary.active ? 3000 : 30000;
    } catch (_) { /* Connection state is handled by the next dashboard refresh. */ }
    finally {
      model.polling = false;
      if (!document.hidden) schedulePoll(nextDelay);
    }
  }

  function schedulePoll(delay = 0) {
    if (model.pollTimer) window.clearTimeout(model.pollTimer);
    model.pollTimer = 0;
    if (document.hidden) return;
    model.pollTimer = window.setTimeout(() => {
      model.pollTimer = 0;
      void poll();
    }, delay);
  }

  const showLogin = (message = '请输入由管理员提供的令牌以继续。') => {
    if (!lanMode) return;
    if (model.pollTimer) window.clearTimeout(model.pollTimer);
    model.pollTimer = 0;
    Remote.deactivate();
    q('.app-shell').hidden = true;
    q('#lan-login').hidden = false;
    q('#lan-login-message').textContent = message;
    q('#lan-token').value = '';
    document.body.classList.remove('auth-pending');
    window.setTimeout(() => q('#lan-token').focus(), 0);
  };

  const startConsole = () => {
    q('#lan-login').hidden = true;
    q('.app-shell').hidden = false;
    q('#lan-logout').hidden = !lanMode;
    document.body.classList.remove('auth-pending');
    if (!initialized) {
      initialized = true;
      selectPackage(model.selectedPackage);
      openView(location.hash.slice(1) || 'overview');
    }
    void loadDashboard().finally(() => schedulePoll(3000));
    void loadApps(true).catch(() => {});
  };

  q('#lan-login-form').addEventListener('submit', async event => {
    event.preventDefault();
    const input = q('#lan-token');
    const button = q('#lan-login-submit');
    const token = input.value;
    q('#lan-login-message').textContent = '';
    setButtonBusy(button, true);
    try {
      const response = await fetch('/v1/auth/lan/session', {
        method: 'POST',
        credentials: 'same-origin',
        headers: {'Accept': 'application/json', 'Content-Type': 'application/json'},
        body: JSON.stringify({token})
      });
      if (!response.ok) {
        showLogin('令牌无效或已过期，请重试。');
        return;
      }
      startConsole();
    } catch (error) {
      showLogin(error.message || '登录失败，请检查连接后重试。');
    } finally {
      input.value = '';
      setButtonBusy(button, false);
    }
  });

  q('#lan-logout').addEventListener('click', async () => {
    try {
      await fetch('/v1/auth/lan/session', {method: 'DELETE', credentials: 'same-origin', headers: {'Accept': 'application/json'}});
    } finally {
      showLogin('已退出登录，请输入由管理员提供的令牌以继续。');
    }
  });

  window.addEventListener('nova-auth-required', () => showLogin('登录已过期，请重新输入访问令牌。'));

  void (async () => {
    try {
      const response = await fetch('/v1/auth/lan/status', {credentials: 'same-origin', headers: {'Accept': 'application/json'}});
      /** @type {LANAuthStatus} */
      const status = response.ok ? await response.json() : {};
      lanMode = status.enabled === true;
      if (lanMode && status.authenticated !== true) {
        showLogin();
        return;
      }
    } catch (_) {
      lanMode = false;
    }
    startConsole();
  })();
})();
