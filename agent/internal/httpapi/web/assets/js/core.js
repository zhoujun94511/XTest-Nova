(() => {
  'use strict';

  const q = selector => document.querySelector(selector);
  const qa = selector => [...document.querySelectorAll(selector)];
  const el = (tag, options = {}) => {
    const node = document.createElement(tag);
    if (options.className) node.className = options.className;
    if (options.text !== undefined) node.textContent = String(options.text);
    if (options.attrs) Object.entries(options.attrs).forEach(([name, value]) => node.setAttribute(name, String(value)));
    return node;
  };

  let pendingRequests = 0;
  const setBusy = busy => {
    pendingRequests = Math.max(0, pendingRequests + (busy ? 1 : -1));
    q('#global-progress').hidden = pendingRequests === 0;
    document.body.classList.toggle('is-busy', pendingRequests > 0);
  };

  async function api(path, options = {}) {
    const {uiSilent = false, ...requestOptions} = options;
    if (!uiSilent) setBusy(true);
    try {
      const response = await fetch(path, {credentials: 'same-origin', ...requestOptions, headers: {'Accept': 'application/json', 'X-XTest-Control': 'true', ...(requestOptions.headers || {})}});
      const contentType = response.headers.get('content-type') || '';
      const body = contentType.includes('json') ? await response.json() : await response.text();
      if (!response.ok) {
        if (response.status === 401) window.dispatchEvent(new CustomEvent('nova-auth-required'));
        const detail = typeof body === 'object' && body ? body.error : body;
        throw new Error(detail || `请求失败 (${response.status})`);
      }
      return body;
    } finally {
      if (!uiSilent) setBusy(false);
    }
  }

  const jsonBody = value => ({
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify(value)
  });

  const required = (selector, message = '请先填写必填项') => {
    const node = q(selector);
    const value = node.value.trim();
    if (!value) {
      node.focus();
      throw new Error(message);
    }
    return value;
  };

  const numberValue = selector => Number(q(selector).value);
  const checked = selector => q(selector).checked;
  const fileURL = path => `/raw/${String(path).replace(/^\/+/, '').split('/').map(encodeURIComponent).join('/')}`;
  const fileInfoURL = path => `/finfo/${String(path).replace(/^\/+/, '').split('/').map(encodeURIComponent).join('/')}`;
  const archiveURL = path => `/archive/${String(path).replace(/^\/+/, '').split('/').map(encodeURIComponent).join('/')}`;
  const formatBytes = value => {
    const bytes = Number(value);
    if (!Number.isFinite(bytes) || bytes < 0) return '—';
    const units = ['B', 'KB', 'MB', 'GB', 'TB'];
    let amount = bytes;
    let index = 0;
    while (amount >= 1024 && index < units.length - 1) { amount /= 1024; index += 1; }
    return `${amount >= 10 || index === 0 ? amount.toFixed(0) : amount.toFixed(1)} ${units[index]}`;
  };
  const formatDuration = millis => {
    let seconds = Math.max(0, Math.floor(Number(millis) / 1000));
    const days = Math.floor(seconds / 86400); seconds %= 86400;
    const hours = Math.floor(seconds / 3600); seconds %= 3600;
    const minutes = Math.floor(seconds / 60);
    if (days) return `${days}天 ${hours}小时`;
    if (hours) return `${hours}小时 ${minutes}分钟`;
    if (minutes) return `${minutes}分钟`;
    return `${seconds}秒`;
  };
  const formatDate = value => {
    if (!value) return '—';
    const date = new Date(value);
    return Number.isNaN(date.getTime()) ? String(value) : new Intl.DateTimeFormat('zh-CN', {month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit'}).format(date);
  };
  const labels = Object.freeze({
    agent: 'Agent 主服务', runtimeBootstrap: '运行环境引导', runnerPayload: '遍历执行器', companionPayload: '悬浮窗组件',
    uiautomatorHostPayload: '控件树宿主', uiautomatorTestPayload: '控件树执行组件', monitor: '异常监控', popupOverlayStartup: '悬浮窗启动',
    autoPopup: '悬浮窗服务', minitouch: '触控注入', uiautomator: '控件树服务', scrcpy: '屏幕控制', screenRecord: '屏幕录制',
    explorationSession: '智能探索会话', runnerSession: '随机遍历会话', companion: '悬浮窗应用',
    running: '运行状态', events: '事件数', crashCount: 'Crash 数量', anrCount: 'ANR 数量', nativeCrashCount: 'Native Crash 数量',
    diagnosticsComplete: '诊断采集完成', steps: '执行步骤', discoveredStates: '发现状态', discoveredEdges: '状态连线', activityCovered: '覆盖页面',
    effectiveActions: '有效动作', ineffectiveActions: '无效动作', externalActions: '跨应用动作', staleActions: '过期动作', cycleDetections: '循环识别',
    package: '应用包名', rows: '数据行数', path: '产物路径', error: '错误', name: '名称', actions: '动作数',
    packageName: '应用包名', versionName: '版本名称', versionCode: '版本号', mainActivity: '启动页面', apkPath: 'APK 路径', system: '系统应用',
    completedActions: '已完成动作', uptime: '运行时长', goroutines: '协程数', heap: '堆内存', heapObjects: '堆对象数', systemMemory: '系统内存', gcCycles: 'GC 次数',
    brand: '品牌', model: '型号', version: '系统版本', arch: '处理器架构', battery: '电池', memory: '内存', storage: '存储', serial: '设备序列号', sdk: 'SDK 版本'
  });
  const humanize = value => labels[value] || String(value || '').replace(/([a-z0-9])([A-Z])/g, '$1 $2').replace(/[_-]+/g, ' ');

  const toast = (message, type = 'success') => {
    const region = q('#toast-region');
    const node = el('div', {className: `toast ${type}`, text: message, attrs: {role: type === 'error' ? 'alert' : 'status'}});
    region.append(node);
    window.setTimeout(() => node.remove(), type === 'error' ? 6000 : 3200);
  };

  const setButtonBusy = (button, busy) => {
    if (!button) return;
    if (busy) {
      button.dataset.label = button.textContent;
      button.textContent = '处理中…';
      button.disabled = true;
    } else {
      button.textContent = button.dataset.label || button.textContent;
      button.disabled = false;
      delete button.dataset.label;
    }
  };

  const appendJSONDetails = (parent, value) => {
    const details = el('details', {className: 'json-details'});
    details.append(el('summary', {text: '查看原始数据'}));
    details.append(el('pre', {text: JSON.stringify(value, null, 2)}));
    parent.append(details);
  };

  window.NovaCore = Object.freeze({q, qa, el, api, jsonBody, required, numberValue, checked, fileURL, fileInfoURL, archiveURL, formatBytes, formatDuration, formatDate, humanize, toast, setButtonBusy, appendJSONDetails});
})();
