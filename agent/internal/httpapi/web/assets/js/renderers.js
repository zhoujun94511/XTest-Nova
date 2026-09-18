(() => {
  'use strict';
  /**
   * JSON models returned by Nova APIs. Keeping these definitions beside the
   * renderers makes API drift visible to static analysis without a build step.
   * @typedef {Object} SessionState
   * @property {boolean} [running]
   * @property {string} [error]
   * @property {boolean} [finalizing]
   * @property {boolean} [stopping]
   */
  /**
   * @typedef {Object} DeviceInfo
   * @property {string} [brand]
   * @property {string} [model]
   * @property {string} [serial]
   * @property {string} [udid]
   * @property {string} [version]
   * @property {string} [arch]
   * @property {{level?: number, usbPowered?: boolean}} [battery]
   * @property {{available?: number, total?: number}} [memory]
   * @property {{available?: number, total?: number}} [storage]
   */
  /**
   * @typedef {Object} ComponentStatus
   * @property {string} name
   * @property {boolean} [ready]
   * @property {boolean} [running]
   * @property {boolean} [supported]
   * @property {string} [state]
   * @property {string} [version]
   * @property {string} [detail]
   */
  /**
   * @typedef {{components?: ComponentStatus[]}} ComponentPayload
   */
  /**
   * @typedef {Object} RecordedCase
   * @property {string} id
   * @property {string} name
   * @property {string} package
   * @property {string} recordedAt
   * @property {number} actions
   */
  /**
   * @typedef {Object} ArtifactSession
   * @property {string} kind
   * @property {string} package
   * @property {string} modified
   * @property {string} name
   * @property {{name: string, size: number, path: string}[]} [files]
   */
  /**
   * @typedef {{sessions?: ArtifactSession[]}} ArtifactPayload
   */
  /**
   * @typedef {Object} RuntimeInfo
   * @property {number} [uptimeMillis]
   * @property {number} [heapAllocBytes]
   * @property {number} [systemBytes]
   */
  const {q, el, formatBytes, formatDuration, formatDate, humanize, fileURL, appendJSONDetails} = window.NovaCore;
  // noinspection SpellCheckingInspection
  const coreComponentNames = ['agent', 'runtimeBootstrap', 'runnerPayload', 'companionPayload', 'uiautomatorHostPayload', 'uiautomatorTestPayload', 'monitor', 'popupOverlayStartup'];
  // noinspection SpellCheckingInspection
  const onDemandComponentNames = new Set(['autoPopup', 'minitouch', 'uiautomator']);

  /** @param {ComponentStatus} item */
  const componentState = item => {
    if (item.ready) return item.running ? '运行中' : '已就绪';
    if (onDemandComponentNames.has(item.name) && ['idle', 'stopped', ''].includes(item.state || '')) return '按需启动';
    if (!item.supported) return '不受支持';
    return item.state === 'degraded' ? '异常' : '未就绪';
  };

  /** @param {ComponentStatus} item */
  const componentDetail = item => {
    const state = componentState(item);
    if (state === '按需启动') return '当前空闲，相关功能使用时自动启动';
    if (state === '运行中') return '组件正在运行';
    if (state === '已就绪') return item.version ? `已就绪 · ${item.version}` : '组件已就绪';
    return item.detail ? `需要检查 · ${item.detail}` : state;
  };

  /**
   * @param {string} selector
   * @param {SessionState|null|undefined} state
   */
  const setPill = (selector, state) => {
    const pill = q(selector);
    const running = Boolean(state && state.running);
    const failed = Boolean(state && state.error);
    pill.textContent = failed ? '异常' : running ? '运行中' : state && (state.finalizing || state.stopping) ? '收尾中' : '空闲';
    pill.className = `state-pill ${failed ? 'error' : running ? 'running' : ''}`.trim();
  };

  const renderData = (selector, value, preferred = []) => {
    const host = q(selector);
    host.replaceChildren();
    if (!value || typeof value !== 'object') {
      host.append(el('p', {className: 'empty-state', text: value || '暂无数据'}));
      return;
    }
    const entries = Object.entries(value).filter(([key, item]) => preferred.includes(key) || (preferred.length === 0 && ['string', 'number', 'boolean'].includes(typeof item)));
    const grid = el('div', {className: 'data-grid'});
    entries.slice(0, 12).forEach(([key, item]) => {
      const card = el('div', {className: 'data-item'});
      card.append(el('span', {text: humanize(key)}));
      card.append(el('strong', {text: typeof item === 'boolean' ? (item ? '是' : '否') : item ?? '—', attrs: {title: String(item ?? '')}}));
      grid.append(card);
    });
    if (entries.length) host.append(grid);
    appendJSONDetails(host, value);
  };

  /** @param {DeviceInfo} device */
  const renderDevice = device => {
	const identity = [device.model, device.serial || device.udid].filter(Boolean).join(' · ') || 'Android 设备';
	q('#device-context').textContent = identity;
	q('#device-context').title = `当前设备：${identity}`;
    const metrics = [
      ['测试设备', [device.brand, device.model].filter(Boolean).join(' ') || 'Android 设备', `Android ${device.version || '—'} · ${device.arch || '—'}`],
      ['电量', `${device.battery?.level ?? '—'}%`, device.battery?.usbPowered ? 'USB 供电中' : '使用电池'],
      ['可用内存', formatBytes((device.memory?.available || 0) * 1024), `共 ${formatBytes((device.memory?.total || 0) * 1024)}`],
      ['可用存储', formatBytes(device.storage?.available), `共 ${formatBytes(device.storage?.total)}`]
    ];
    const host = q('#device-metrics');
    host.replaceChildren();
    metrics.forEach(([label, value, hint]) => {
      const card = el('article', {className: 'metric-card'});
      card.append(el('span', {text: label}), el('strong', {text: value, attrs: {title: value}}), el('small', {text: hint}));
      host.append(card);
    });
    const details = q('#device-details');
    details.replaceChildren();
    Object.entries(device).forEach(([key, value]) => {
      const row = el('div');
      row.append(el('dt', {text: humanize(key)}), el('dd', {text: typeof value === 'object' ? JSON.stringify(value) : value}));
      details.append(row);
    });
  };

  /** @param {ComponentPayload|null|undefined} payload */
  const renderReadiness = payload => {
    const components = payload?.components || [];
    const selected = coreComponentNames.map(name => components.find(item => item.name === name)).filter(Boolean);
    const ready = selected.filter(item => item.ready);
    q('#readiness-score').textContent = `${ready.length}/${selected.length || coreComponentNames.length}`;
    const host = q('#readiness-list');
    host.replaceChildren();
    selected.forEach(item => {
      const row = el('div', {className: 'status-row'});
      row.append(el('span', {text: humanize(item.name)}), el('strong', {className: item.ready ? 'ready' : 'warning', text: componentState(item)}));
      host.append(row);
    });
    if (!selected.length) host.append(el('p', {className: 'empty-state', text: '尚未读取核心组件状态。'}));
  };

  /** @param {ComponentPayload|null|undefined} payload */
  const renderComponents = payload => {
    const host = q('#components-output');
    host.replaceChildren();
    const components = payload?.components || [];
    const counts = {ready: 0, idle: 0, degraded: 0};
    components.forEach(item => {
      const state = componentState(item);
      const kind = item.ready ? 'ready' : state === '按需启动' ? 'idle' : 'degraded';
      counts[kind] += 1;
      const row = el('div', {className: `component-row ${kind}`});
      const info = el('div');
      info.append(el('strong', {text: humanize(item.name)}));
      info.append(el('small', {text: componentDetail(item)}));
      row.append(info, el('span', {className: `component-state ${kind}`, text: state}));
      host.append(row);
    });
    const summary = document.querySelector('#component-summary');
    if (summary) {
      summary.replaceChildren();
      [['ready', '正常'], ['idle', '按需启动'], ['degraded', '需处理']].forEach(([kind, label]) => {
        const chip = el('span', {className: `component-summary-chip ${kind}`});
        chip.append(el('strong', {text: counts[kind]}), el('span', {text: label}));
        summary.append(chip);
      });
    }
    if (!components.length) host.append(el('p', {className: 'empty-state', text: '尚未读取组件状态。'}));
  };

  const renderApps = (packages, selected) => {
    const host = q('#package-list');
    host.replaceChildren();
    q('#app-count').textContent = `${packages.length} 个应用`;
    if (!packages.length) { host.append(el('p', {className: 'empty-state', text: '没有找到匹配的应用。'})); return; }
    packages.forEach(item => {
      const packageName = typeof item === 'string' ? item : item.packageName;
      const name = typeof item === 'object' ? item.name || item.label || packageName : packageName;
      const button = el('button', {className: `app-row ${packageName === selected ? 'selected' : ''}`, attrs: {type: 'button', 'data-package': packageName}});
      const identity = el('span', {className: 'app-identity'});
      const avatar = el('span', {className: 'app-avatar', text: packageName.slice(0, 1).toUpperCase()});
      const icon = el('img', {className: 'app-icon', attrs: {src: `/packages/${encodeURIComponent(packageName)}/icon`, alt: '', loading: 'lazy'}});
      icon.addEventListener('load', () => avatar.classList.add('has-icon'));
      icon.addEventListener('error', () => icon.remove());
      avatar.append(icon);
      const copy = el('span');
      copy.append(el('strong', {text: name}));
      if (name !== packageName) copy.append(el('small', {text: packageName}));
      identity.append(avatar, copy);
      button.append(identity, el('small', {text: packageName === selected ? '已选择' : '选择'}));
      host.append(button);
    });
  };

  const renderAppDetails = payload => {
    const value = payload?.data || payload;
	renderData('#app-output', value, ['name', 'chineseName', 'englishName', 'packageName', 'versionName', 'versionCode', 'mainActivity', 'apkPath', 'system']);
  };

  /**
   * @param {RecordedCase[]} cases
   * @param {string} selectedID
   */
  const renderCases = (cases, selectedID) => {
    const host = q('#record-list-output');
    host.replaceChildren();
    if (!cases.length) { host.append(el('p', {className: 'empty-state', text: '当前目标没有已保存用例。'})); return; }
    cases.forEach(item => {
      const button = el('button', {className: `case-row ${item.id === selectedID ? 'selected' : ''}`, attrs: {type: 'button', 'data-case-id': item.id}});
      const label = el('span'); label.append(el('strong', {text: item.name}), el('small', {text: `${item.package} · ${formatDate(item.recordedAt)}`}));
      button.append(label, el('small', {text: `${item.actions} 步`}));
      host.append(button);
    });
  };

  /** @param {ArtifactPayload|null|undefined} payload */
  const renderArtifacts = payload => {
    const host = q('#artifacts-output');
    host.replaceChildren();
    const sessions = payload?.sessions || [];
    if (!sessions.length) { host.append(el('p', {className: 'empty-state', text: '没有符合条件的测试产物。'})); return; }
    sessions.forEach(session => {
      const card = el('article', {className: 'artifact-card'});
      const head = el('div', {className: 'artifact-head'});
      const title = el('div'); title.append(el('strong', {text: `${session.kind} · ${session.package}`}), el('span', {text: `${formatDate(session.modified)} · ${session.name}`}));
      head.append(title, el('span', {text: `${(session.files || []).length} 个文件`}));
      const files = el('div', {className: 'artifact-files'});
      // noinspection SpellCheckingInspection
      (session.files || []).forEach(file => files.append(el('a', {text: `${file.name} · ${formatBytes(file.size)}`, attrs: {href: fileURL(file.path), target: '_blank', rel: 'noopener'}})));
      card.append(head, files); host.append(card);
    });
  };

  const renderFileTree = entry => {
    q('#file-breadcrumb').textContent = entry.path || '—';
    const host = q('#file-tree');
    host.replaceChildren();
    const children = [...(entry.files || [])].sort((left, right) => Number(right.isDirectory) - Number(left.isDirectory) || left.name.localeCompare(right.name, 'zh-CN'));
    if (!children.length) {
      host.append(el('p', {className: 'empty-state', text: '当前目录为空。'}));
      return;
    }
    children.forEach(item => {
      const button = el('button', {className: 'file-row', attrs: {type: 'button', 'data-file-path': item.path, 'data-file-directory': item.isDirectory, 'data-file-size': item.size || 0}});
      button.append(el('span', {className: 'file-kind', text: item.isDirectory ? '▸' : '·'}));
      const identity = el('span', {className: 'file-identity'});
      identity.append(el('strong', {text: item.name}), el('small', {text: item.isDirectory ? '文件夹' : formatBytes(item.size)}));
      button.append(identity, el('span', {className: 'file-action', text: item.isDirectory ? '打开' : '选择'}));
      host.append(button);
    });
  };

  const renderFilePreview = preview => {
    const host = q('#file-preview');
    host.hidden = false;
    host.replaceChildren();
    if (preview.message) {
      host.append(el('p', {className: 'empty-state', text: preview.message}));
      return;
    }
    if (preview.kind === 'image') host.append(el('img', {attrs: {src: preview.url, alt: preview.name || '文件预览'}}));
    else if (preview.kind === 'audio') host.append(el('audio', {attrs: {src: preview.url, controls: true, preload: 'metadata'}}));
    else if (preview.kind === 'video') host.append(el('video', {attrs: {src: preview.url, controls: true, preload: 'metadata'}}));
    else if (preview.kind === 'pdf') host.append(el('iframe', {attrs: {src: preview.url, title: preview.name || 'PDF 预览'}}));
    else if (preview.kind === 'text') host.append(el('pre', {text: preview.text || ''}));
  };

  /** @param {RuntimeInfo|null|undefined} value */
  const renderRuntime = value => {
    const normalized = {...value, uptime: formatDuration(value?.uptimeMillis), heap: formatBytes(value?.heapAllocBytes), systemMemory: formatBytes(value?.systemBytes)};
    renderData('#diagnostics-output', normalized, ['uptime', 'goroutines', 'heap', 'heapObjects', 'systemMemory', 'gcCycles']);
    const updated = document.querySelector('#diagnostics-updated');
    if (updated) updated.textContent = `更新于 ${new Intl.DateTimeFormat('zh-CN', {hour: '2-digit', minute: '2-digit', second: '2-digit'}).format(new Date())}`;
  };

  window.NovaRenderers = Object.freeze({setPill, renderData, renderDevice, renderReadiness, renderComponents, renderApps, renderAppDetails, renderCases, renderArtifacts, renderFileTree, renderFilePreview, renderRuntime});
})();
