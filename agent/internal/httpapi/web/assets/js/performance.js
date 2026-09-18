(() => {
  'use strict';
  /**
   * JSON model returned by the performance session API.
   * @typedef {Object} PerformanceCPU
   * @property {number} [percent]
   * @property {number} [corePercent]
   * @property {number} [systemPercent]
   * @property {number} [coreCount]
   * @property {string} [scope]
   * @property {number[]} [pids]
   */
  /**
   * @typedef {Object} PerformanceBattery
   * @property {number} [levelPercent]
   * @property {number} [temperatureC]
   * @property {number|null} [currentMa]
   * @property {string} [status]
   */
  /**
   * @typedef {Object} PerformanceNetwork
   * @property {number} [rx]
   * @property {number} [tx]
   * @property {number} [rxBytesPerSecond]
   * @property {number} [txBytesPerSecond]
   * @property {number} [rateWindowMillis]
   * @property {string} [rateSampledAt]
   * @property {number} [uid]
   * @property {string} [scope]
   * @property {string} [source]
   */
  /**
   * @typedef {Object} PerformanceFrame
   * @property {number} jankCount
   * @property {number} jankRate
   * @property {string} [source]
   */
  /**
   * @typedef {Object} PerformanceSample
   * @property {PerformanceCPU} [cpuinfo]
   * @property {Object.<string, number>} [memoinfo]
   * @property {number|null} [fps]
   * @property {number|null} [renderFps]
   * @property {PerformanceFrame|null} [frame]
   * @property {{percent: number}|null} [gpu]
   * @property {PerformanceBattery} [battery]
   * @property {PerformanceNetwork} [network]
   * @property {Object.<string, string>} [sources]
   * @property {Object.<string, string>} [errors]
   * @property {Object.<string, {state: string, source?: string, reason?: string}>} [metricStates]
   * @property {number} [collectionDurationMillis]
   * @property {string} [collectedAt]
   */
  /**
   * @typedef {Object} PerformanceState
   * @property {boolean} [running]
   * @property {number} [rows]
   * @property {number} [failedSamples]
   * @property {number} [partialSamples]
   * @property {number} [intervalSeconds]
   * @property {string} [startedAt]
   * @property {string} [path]
   * @property {string} [sessionPath]
   * @property {string} [summaryPath]
   * @property {PerformanceSample|null} [last]
   */
  /**
   * @typedef {Object} PerformanceWarning
   * @property {string} metric
   * @property {number} [validSamples]
   * @property {number} [totalSamples]
   * @property {number} [coveragePercent]
   */
  /**
   * @typedef {Object} PerformanceQualityCounts
   * @property {number} [measured]
   * @property {number} [idle]
   * @property {number} [warmingUp]
   * @property {number} [unsupported]
   * @property {number} [failed]
   */
  /**
   * @typedef {Object} PerformanceSummary
   * @property {Object.<string, Object.<string, number>>} [metrics]
   * @property {Object.<string, PerformanceQualityCounts>} [metricQuality]
   * @property {PerformanceWarning[]} [warnings]
   */
  const {q, el, formatBytes, formatDuration, formatDate, fileURL} = window.NovaCore;
  const history = [];
  let lastTimestamp = '';

  const number = (value, digits = 1, suffix = '') => Number.isFinite(Number(value)) ? `${Number(value).toFixed(digits)}${suffix}` : '—';
  /** @param {PerformanceSample|null|undefined} sample */
  const memoryKB = sample => sample?.memoinfo?.['total pss'] || sample?.memoinfo?.total || 0;
  /**
   * @param {PerformanceSample|null|undefined} sample
   * @param {string} name
   */
  const source = (sample, name) => sample?.sources?.[name] || '来源不可用';
  const stateText = (sample, name, value, idleText = '') => {
    const state = sample?.metricStates?.[name]?.state || '';
    if (state === 'warming_up') return '预热中';
    if (state === 'unsupported') return '设备不支持';
    if (state === 'failed') return '采集失败';
    if (state === 'idle') return idleText || `${value} · 空闲`;
    return value;
  };
  const stateDetail = (sample, name, fallback) => sample?.metricStates?.[name]?.reason || fallback;

  const metric = (label, value, detail) => {
    const card = el('article', {className: 'performance-metric'});
    card.append(el('span', {text: label}), el('strong', {text: value}), el('small', {text: detail || '—'}));
    return card;
  };

  /** @param {PerformanceSample|null|undefined} sample */
  const renderMetrics = sample => {
    const host = q('#perf-metrics');
    host.replaceChildren();
    if (!sample) {
      host.append(el('p', {className: 'empty-state', text: '等待首个性能样本。'}));
      return;
    }
    const cpu = sample.cpuinfo || {};
    const battery = sample.battery || {};
    const network = sample.network || {};
	const networkRateState = sample.metricStates?.network_rate?.state || '';
	const networkRate = networkRateState === 'warming_up'
	  ? '速率预热中'
	  : networkRateState === 'failed'
	    ? '速率采集失败'
	    : `↓ ${formatBytes(network.rxBytesPerSecond || 0)}/s · ↑ ${formatBytes(network.txBytesPerSecond || 0)}/s`;
    host.append(
      metric('应用 CPU', stateText(sample, 'cpu', number(cpu.corePercent, 1, '%')), `${number(cpu.percent, 1, '%')} 整机容量 · ${cpu.scope === 'package_processes' ? `${cpu.pids?.length || 1} 个应用进程` : cpu.scope === 'main_process' ? '主进程' : cpu.scope || '范围未知'}`),
      metric('系统 CPU', stateText(sample, 'cpu', number(cpu.systemPercent, 1, '%')), `${cpu.coreCount || '—'} 核 · ${source(sample, 'cpu')}`),
      metric('应用内存', stateText(sample, 'memory', memoryKB(sample) ? formatBytes(memoryKB(sample) * 1024) : '—'), stateDetail(sample, 'memory', source(sample, 'memory'))),
      metric('屏幕呈现帧率', stateText(sample, 'fps', sample.fps == null ? '—' : number(sample.fps, 1, ' FPS'), '静止（本窗口无新呈现帧）'), stateDetail(sample, 'fps', source(sample, 'fps'))),
      metric('View 渲染率', stateText(sample, 'render_rate', sample.renderFps == null ? '—' : number(sample.renderFps, 1, ' 帧/秒'), '静止（本窗口无新渲染帧）'), stateDetail(sample, 'render_rate', source(sample, 'renderRate'))),
      metric('卡顿', stateText(sample, 'jank', sample.frame ? `${sample.frame.jankCount} 帧 · ${number(sample.frame.jankRate, 1, '%')}` : '—', '无新帧，不计算卡顿率'), stateDetail(sample, 'jank', sample.frame?.source || '等待有效帧窗口')),
      metric('GPU', stateText(sample, 'gpu', sample.gpu ? number(sample.gpu.percent, 1, '%') : '—'), stateDetail(sample, 'gpu', source(sample, 'gpu'))),
      metric('电池与温度', `${battery.levelPercent ?? '—'}% · ${number(battery.temperatureC, 1, '℃')}`, stateText(sample, 'battery_current', battery.currentMa == null ? '电流不可用' : `${number(battery.currentMa, 1, ' mA')} · ${battery.status || '状态未知'}`)),
      metric('应用网络', network.source ? networkRate : '—', network.scope === 'package_uid' ? `累计 ↓ ${formatBytes(network.rx)} · ↑ ${formatBytes(network.tx)} · UID ${network.uid}` : '应用级数据不可用'),
      metric('采样开销', `${sample.collectionDurationMillis ?? 0} ms`, formatDate(sample.collectedAt))
    );
  };

  /** @param {PerformanceSample|null|undefined} sample */
  const addHistory = sample => {
    if (!sample?.collectedAt || sample.collectedAt === lastTimestamp) return;
    lastTimestamp = sample.collectedAt;
    history.push({cpu: sample.metricStates?.cpu?.state === 'warming_up' ? NaN : Number(sample.cpuinfo?.corePercent), fps: sample.metricStates?.fps?.state === 'measured' && sample.fps != null ? Number(sample.fps) : NaN, memory: memoryKB(sample) / 1024, rx: Number(sample.network?.rxBytesPerSecond), tx: Number(sample.network?.txBytesPerSecond)});
    if (history.length > 120) history.shift();
  };

  const sparkline = (label, key, unit, color) => {
    const values = history.map(item => item[key]).filter(Number.isFinite);
    if (!values.length) return null;
    const minimum = Math.min(...values), maximum = Math.max(...values), span = Math.max(maximum - minimum, 1);
    const width = 320, height = 72;
    const points = history.map((item, index) => {
      const value = item[key];
      if (!Number.isFinite(value)) return null;
      const x = history.length === 1 ? width : index * width / (history.length - 1);
      const y = height - ((value - minimum) / span * (height - 8) + 4);
      return `${x.toFixed(1)},${y.toFixed(1)}`;
    }).filter(Boolean).join(' ');
    const row = el('article', {className: 'performance-trend'});
    const header = el('div');
    header.append(el('strong', {text: label}), el('span', {text: `${minimum.toFixed(1)}–${maximum.toFixed(1)} ${unit}`}));
    const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
    svg.setAttribute('viewBox', `0 0 ${width} ${height}`); svg.setAttribute('role', 'img'); svg.setAttribute('aria-label', `${label}趋势`);
    const line = document.createElementNS('http://www.w3.org/2000/svg', 'polyline');
    line.setAttribute('points', points); line.setAttribute('fill', 'none'); line.setAttribute('stroke', color); line.setAttribute('stroke-width', '3'); line.setAttribute('vector-effect', 'non-scaling-stroke');
    svg.append(line); row.append(header, svg); return row;
  };

  const renderTrends = () => {
    const host = q('#perf-trends'); host.replaceChildren();
    [['CPU', 'cpu', '%', '#67dfbb'], ['FPS', 'fps', 'FPS', '#78a9ff'], ['内存', 'memory', 'MB', '#d7a8ff'], ['网络下行', 'rx', 'B/s', '#ffc66d'], ['网络上行', 'tx', 'B/s', '#ff8fab']].forEach(config => {
      const chart = sparkline(...config); if (chart) host.append(chart);
    });
  };

  /** @param {PerformanceSample|null|undefined} sample */
  const renderErrors = sample => {
    const host = q('#perf-errors');
    const entries = Object.entries(sample?.errors || {});
    host.hidden = entries.length === 0; host.replaceChildren();
    if (entries.length) {
      host.append(el('strong', {text: '部分指标暂不可用'}));
      entries.forEach(([name, message]) => host.append(el('p', {text: `${name}：${message}`})));
    }
  };

  /** @param {PerformanceState} state */
  const renderState = state => {
    const host = q('#perf-output'); host.replaceChildren();
    const grid = el('div', {className: 'data-grid'});
    [['状态', state.running ? '采集中' : '已停止'], ['样本', state.rows ?? 0], ['失败采样', state.failedSamples ?? 0], ['部分样本', state.partialSamples ?? 0], ['采样间隔', `${state.intervalSeconds || 1} 秒`], ['持续时间', state.startedAt ? formatDuration(Date.now() - new Date(state.startedAt).getTime()) : '—']].forEach(([label, value]) => {
      const card = el('div', {className: 'data-item'}); card.append(el('span', {text: label}), el('strong', {text: value})); grid.append(card);
    });
    host.append(grid);
    const links = el('div', {className: 'performance-artifacts'});
    [['原始 CSV', state.path, true], ['会话信息', state.sessionPath, true], ['统计摘要', state.summaryPath, !state.running]].forEach(([label, path, available]) => {
      if (path && available) links.append(el('a', {text: label, attrs: {href: fileURL(path), target: '_blank', rel: 'noopener'}}));
    });
    if (links.childElementCount) host.append(links);
  };

  /** @param {PerformanceSummary|null|undefined} summary */
  const renderSummary = summary => {
    const host = q('#perf-summary'); host.replaceChildren();
    if (!summary?.metrics) { host.hidden = true; return; }
    host.hidden = false;
    host.append(el('h3', {text: '本次采集摘要'}));
    const grid = el('div', {className: 'data-grid'});
    const definitions = [
      ['CPU P95', 'app_cpu_core_percent', 'p95', '%'], ['内存 P95', 'memory_kb', 'p95', ' KB'], ['呈现 FPS P50', 'fps', 'p50', ' FPS'], ['View 渲染率 P50', 'render_fps', 'p50', ' 帧/秒'],
      ['卡顿率 P95', 'jank_rate_percent', 'p95', '%'], ['网络下行 P95', 'network_rx_bytes_per_second', 'p95', ' B/s'], ['网络上行 P95', 'network_tx_bytes_per_second', 'p95', ' B/s']
    ];
    const warningMetrics = new Set((summary.warnings || []).map(warning => warning.metric));
    definitions.forEach(([label, key, field, unit]) => {
      const metricSummary = summary.metrics[key];
      const value = warningMetrics.has(key) ? '样本不足' : metricSummary ? `${Number(metricSummary[field]).toFixed(1)}${unit}` : '无有效样本';
      const card = el('div', {className: 'data-item'}); card.append(el('span', {text: label}), el('strong', {text: value})); grid.append(card);
    });
    host.append(grid);
    const quality = el('div', {className: 'performance-quality'});
    const names = {cpu: 'CPU', memory: '内存', fps: '呈现帧率', render_rate: 'View 渲染率', jank: '卡顿', gpu: 'GPU', battery_current: '电流', network: '网络累计量', network_rate: '网络速率'};
    Object.entries(summary.metricQuality || {}).forEach(([name, counts]) => {
      const item = el('div', {className: 'performance-quality-item'});
      item.append(el('strong', {text: names[name] || name}), el('span', {text: `有效 ${counts.measured || 0} · 空闲 ${counts.idle || 0} · 预热 ${counts.warmingUp || 0} · 不支持 ${counts.unsupported || 0} · 失败 ${counts.failed || 0}`}));
      quality.append(item);
    });
    if (quality.childElementCount) host.append(quality);
    const warnings = summary.warnings || [];
    if (warnings.length) {
      const warningHost = el('div', {className: 'performance-quality'});
      warnings.forEach(warning => {
        const item = el('div', {className: 'performance-quality-item'});
        item.append(el('strong', {text: warning.metric === 'fps' ? '呈现帧率样本不足' : warning.metric === 'jank_rate_percent' ? '卡顿样本不足' : warning.metric}), el('span', {text: `有效 ${warning.validSamples || 0}/${warning.totalSamples || 0}（${Number(warning.coveragePercent || 0).toFixed(1)}%），不建议据此形成结论`}));
        warningHost.append(item);
      });
      host.append(warningHost);
    }
  };

  /** @param {PerformanceState|null|undefined} state */
  const render = state => {
    const sample = state?.last;
    addHistory(sample); renderMetrics(sample); renderTrends(); renderErrors(sample); renderState(state || {});
  };

  const reset = () => {
    history.length = 0; lastTimestamp = '';
    const summary = q('#perf-summary');
    summary.hidden = true; summary.replaceChildren();
  };
  window.NovaPerformanceView = {render, renderSummary, reset};
})();
