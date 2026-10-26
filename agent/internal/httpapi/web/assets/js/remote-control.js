(() => {
  'use strict';

  const q = selector => document.querySelector(selector);
  const startCode = new Uint8Array([0, 0, 0, 1]);
  const keyMap = {Enter: 66, Backspace: 67, Escape: 4, ArrowUp: 19, ArrowDown: 20, ArrowLeft: 21, ArrowRight: 22, Delete: 112, Tab: 61};
  let notify = () => {};
  let active = false;
  let videoSocket = null;
  let controlSocket = null;
  let reconnectTimer = 0;
  let qualityTimer = 0;
  let screenToken = 0;
  let controlToken = 0;
  let sequence = 1;
  let receivedBytes = 0;
  let decoder = null;
  let pointerId = null;

  const setState = (selector, text, kind = '') => {
    const element = q(selector);
    if (!element) return;
    element.textContent = text;
    element.className = `state-pill ${kind}`.trim();
  };

  const formatBytes = value => {
    const units = ['B', 'KB', 'MB', 'GB'];
    let number = value;
    let index = 0;
    while (number >= 1024 && index < units.length - 1) { number /= 1024; index += 1; }
    return `${number.toFixed(index ? 1 : 0)} ${units[index]}`;
  };

  const join = parts => {
    const length = parts.reduce((total, part) => total + part.length, 0);
    const result = new Uint8Array(length);
    let offset = 0;
    parts.forEach(part => { result.set(part, offset); offset += part.length; });
    return result;
  };

  class H264CanvasDecoder {
    constructor(canvas) {
      this.canvas = canvas;
      this.context = canvas.getContext('2d', {alpha: false, desynchronized: true});
      this.buffer = new Uint8Array(0);
      this.prefix = [];
      this.sps = null;
      this.pps = null;
      this.codec = '';
      this.timestamp = 0;
      this.frameTimes = [];
      this.awaitingKey = true;
      if (!window.VideoDecoder) throw new Error('当前浏览器不支持 WebCodecs H.264 解码，请使用最新版 Chrome 或 Edge');
      this.videoDecoder = new VideoDecoder({
        output: frame => this.draw(frame),
        error: error => setState('#remote-video-state', `解码失败：${error.message}`, 'error')
      });
    }

    append(data) {
      this.buffer = join([this.buffer, new Uint8Array(data)]);
      const starts = [];
      for (let index = 0; index < this.buffer.length - 3; index += 1) {
        if (this.buffer[index] !== 0 || this.buffer[index + 1] !== 0) continue;
        if (this.buffer[index + 2] === 1) { starts.push([index, 3]); index += 2; }
        else if (this.buffer[index + 2] === 0 && this.buffer[index + 3] === 1) { starts.push([index, 4]); index += 3; }
      }
      if (!starts.length) {
        if (this.buffer.length > 4 * 1024 * 1024) throw new Error('画面流中未找到 H.264 起始码');
        return;
      }
      for (let index = 0; index < starts.length - 1; index += 1) {
        const begin = starts[index][0] + starts[index][1];
        const end = starts[index + 1][0];
        if (end > begin) this.consume(this.buffer.slice(begin, end));
      }
      this.buffer = this.buffer.slice(starts[starts.length - 1][0]);
    }

    consume(nal) {
      const type = nal[0] & 0x1f;
      if (type === 7) { this.sps = nal; this.configure(); return; }
      if (type === 8) { this.pps = nal; this.configure(); return; }
      if (type !== 1 && type !== 5) { this.prefix.push(nal); return; }
      const key = type === 5;
      if (!this.codec || (this.awaitingKey && !key)) { this.prefix = []; return; }
      const units = [];
      if (key) {
        units.push(startCode, this.sps, startCode, this.pps);
        this.awaitingKey = false;
      }
      this.prefix.forEach(item => units.push(startCode, item));
      units.push(startCode, nal);
      this.prefix = [];
      if (!key && this.videoDecoder.decodeQueueSize > 12) return;
      const timestamp = Math.max(this.timestamp + 1, Math.round(performance.now() * 1000));
      this.timestamp = timestamp;
      this.videoDecoder.decode(new EncodedVideoChunk({type: key ? 'key' : 'delta', timestamp, data: join(units)}));
    }

    configure() {
      if (!this.sps || !this.pps || this.sps.length < 4) return;
      const codec = `avc1.${[this.sps[1], this.sps[2], this.sps[3]].map(value => Number(value).toString(16).padStart(2, '0')).join('')}`;
      if (codec === this.codec) return;
      this.codec = codec;
      this.videoDecoder.configure({codec, optimizeForLatency: true, hardwareAcceleration: 'prefer-hardware'});
    }

    draw(frame) {
      const now = performance.now();
      this.frameTimes.push(now);
      while (this.frameTimes.length && now - this.frameTimes[0] > 1000) this.frameTimes.shift();
      if (this.canvas.width !== frame.displayWidth || this.canvas.height !== frame.displayHeight) {
        this.canvas.width = frame.displayWidth;
        this.canvas.height = frame.displayHeight;
        fitPreview(this.canvas);
      }
      q('#remote-resolution').textContent = `${frame.displayWidth} × ${frame.displayHeight} · ${this.frameTimes.length} 帧/秒`;
      this.context.drawImage(frame, 0, 0, this.canvas.width, this.canvas.height);
      frame.close();
      q('#remote-placeholder').hidden = true;
      setState('#remote-video-state', `画面已连接 · ${qualityLabel()}`, 'running');
    }

    close() {
      if (this.videoDecoder && this.videoDecoder.state !== 'closed') this.videoDecoder.close();
      this.buffer = new Uint8Array(0);
    }
  }

  const socketURL = path => `${location.protocol === 'https:' ? 'wss:' : 'ws:'}//${location.host}${path}`;

  const qualityLabel = () => ({low: '流畅', normal: '均衡', high: '高清', original: '原始'})[q('#remote-quality').value] || '均衡';

  const fitPreview = canvas => {
    const stage = canvas?.parentElement;
    if (!stage || canvas.width < 2 || canvas.height < 2) return;
    const scale = Math.min(stage.clientWidth / canvas.width, stage.clientHeight / canvas.height);
    if (!Number.isFinite(scale) || scale <= 0) return;
    canvas.style.width = `${Math.round(canvas.width * scale)}px`;
    canvas.style.height = `${Math.round(canvas.height * scale)}px`;
  };

  const closeSockets = () => {
    screenToken += 1;
    controlToken += 1;
    if (reconnectTimer) window.clearTimeout(reconnectTimer);
    if (qualityTimer) window.clearTimeout(qualityTimer);
    reconnectTimer = 0;
    qualityTimer = 0;
    const screen = videoSocket;
    const control = controlSocket;
    videoSocket = null;
    controlSocket = null;
    if (screen) screen.close();
    if (control) control.close();
    if (decoder) decoder.close();
    decoder = null;
    pointerId = null;
  };

  const openScreen = () => {
    if (!active || videoSocket) return;
    const token = ++screenToken;
    const quality = q('#remote-quality').value;
    receivedBytes = 0;
    const firstFrame = q('#remote-resolution').textContent === '等待首帧';
    if (firstFrame) {
      q('#remote-placeholder').hidden = false;
      q('#remote-placeholder strong').textContent = '正在连接设备画面';
    }
    setState('#remote-video-state', `正在切换到${qualityLabel()}`);
    try { decoder = new H264CanvasDecoder(q('#remote-canvas')); }
    catch (error) { setState('#remote-video-state', error.message, 'error'); return; }
    const screen = new WebSocket(socketURL(`/scrcpy/screen/${quality}`));
    videoSocket = screen;
    screen.binaryType = 'arraybuffer';
    screen.onmessage = event => {
      if (videoSocket !== screen || token !== screenToken || !(event.data instanceof ArrayBuffer)) return;
      receivedBytes += event.data.byteLength;
      q('#remote-traffic').textContent = `已接收 ${formatBytes(receivedBytes)}`;
      try { decoder.append(event.data); }
      catch (error) { setState('#remote-video-state', error.message, 'error'); }
    };
    screen.onerror = () => {
      if (videoSocket !== screen) return;
      setState('#remote-video-state', '画面连接失败，请重新认证', 'error');
      notify('画面连接失败，请确认登录仍然有效后重新认证', 'error');
    };
    screen.onclose = () => {
      if (videoSocket !== screen) return;
      videoSocket = null;
      if (decoder) { decoder.close(); decoder = null; }
      if (!active || token !== screenToken) return;
      setState('#remote-video-state', '画面重连中');
      reconnectTimer = window.setTimeout(() => { reconnectTimer = 0; openScreen(); }, 1200);
    };
  };

  const openControl = () => {
    if (!active || controlSocket) return;
    const token = ++controlToken;
    setState('#remote-control-state', '控制连接中');
    const control = new WebSocket(socketURL('/scrcpy/control/original'));
    controlSocket = control;
    control.onopen = () => { if (controlSocket === control) setState('#remote-control-state', '控制已连接', 'running'); };
    control.onerror = () => {
      if (controlSocket !== control) return;
      setState('#remote-control-state', '控制连接失败，请重新认证', 'error');
      notify('控制连接失败，请确认登录仍然有效后重新认证', 'error');
    };
    control.onclose = () => {
      if (controlSocket !== control) return;
      controlSocket = null;
      if (!active || token !== controlToken) return;
      setState('#remote-control-state', '控制重连中');
      window.setTimeout(() => { if (active && token === controlToken) openControl(); }, 1200);
    };
    control.onmessage = event => {
      if (controlSocket !== control) return;
      let message;
      try { message = JSON.parse(event.data); } catch { return; }
      if (message.success === false) { notify(message.error || '远程控制失败', 'error'); return; }
      if (message.type === 'clipboard') { q('#remote-text').value = message.text || ''; notify('已读取设备剪贴板'); }
    };
  };

  const connect = () => { openScreen(); openControl(); };

  const replaceScreen = () => {
    if (reconnectTimer) window.clearTimeout(reconnectTimer);
    reconnectTimer = 0;
    screenToken += 1;
    const screen = videoSocket;
    videoSocket = null;
    if (decoder) { decoder.close(); decoder = null; }
    if (screen) screen.close();
    openScreen();
  };

  const send = message => {
    if (!controlSocket || controlSocket.readyState !== WebSocket.OPEN) {
      notify('远程控制通道尚未连接', 'error');
      return false;
    }
    controlSocket.send(JSON.stringify(message));
    return true;
  };

  const point = event => {
    const canvas = q('#remote-canvas');
    const bounds = canvas.getBoundingClientRect();
    return {
      xP: Math.max(0, Math.min(1, (event.clientX - bounds.left) / bounds.width)),
      yP: Math.max(0, Math.min(1, (event.clientY - bounds.top) / bounds.height)),
      screenWidth: canvas.width,
      screenHeight: canvas.height
    };
  };

  const installInteractions = () => {
    const stage = q('#remote-stage');
    const canvas = q('#remote-canvas');
    if (!stage || stage.dataset.remoteReady) return;
    stage.dataset.remoteReady = '1';
    fitPreview(canvas);
    new ResizeObserver(() => fitPreview(canvas)).observe(stage);
    canvas.addEventListener('pointerdown', event => {
      event.preventDefault();
      pointerId = Math.min(9, Math.max(0, event.pointerId || 0));
      canvas.setPointerCapture?.(event.pointerId);
      stage.focus();
      send({type: 0, operation: 'd', index: pointerId, ...point(event)});
    });
    canvas.addEventListener('pointermove', event => {
      if (pointerId === null) return;
      event.preventDefault();
      send({type: 0, operation: 'm', index: pointerId, ...point(event)});
    });
    const release = event => {
      if (pointerId === null) return;
      event.preventDefault();
      send({type: 0, operation: 'u', index: pointerId, ...point(event)});
      pointerId = null;
    };
    canvas.addEventListener('pointerup', release);
    canvas.addEventListener('pointercancel', release);
    canvas.addEventListener('wheel', event => {
      event.preventDefault();
      send({type: 6, ...point(event), hScroll: -Math.sign(event.deltaX) * 2, vScroll: -Math.sign(event.deltaY) * 2});
    }, {passive: false});
    stage.addEventListener('keydown', event => {
      const keycode = keyMap[event.key];
      if (!keycode) return;
      event.preventDefault();
      send({type: 1, keycode});
    });
    q('#remote-quality').addEventListener('change', () => {
      if (!active) return;
      if (qualityTimer) window.clearTimeout(qualityTimer);
      setState('#remote-video-state', `正在切换到${qualityLabel()}`);
      qualityTimer = window.setTimeout(() => { qualityTimer = 0; replaceScreen(); }, 250);
    });
    document.addEventListener('click', handleCommand);
  };

  const handleCommand = event => {
    const key = event.target.closest('[data-remote-key]');
    if (key) { send({type: 1, keycode: Number(key.dataset.remoteKey)}); return; }
    const button = event.target.closest('[data-remote-command]');
    if (!button) return;
    const command = button.dataset.remoteCommand;
    const stage = q('#remote-stage');
    if (command === 'reconnect') { closeSockets(); active = true; connect(); }
    else if (command === 'rotate') send({type: 3});
    else if (command === 'fullscreen') stage.requestFullscreen?.();
    else if (command === 'capture') q('#remote-canvas').toBlob(blob => {
      if (!blob) return;
      const link = document.createElement('a');
      link.href = URL.createObjectURL(blob);
      link.download = `xtest-nova-screen-${Date.now()}.png`;
      link.click();
      window.setTimeout(() => URL.revokeObjectURL(link.href), 1000);
    }, 'image/png');
    else {
      const text = q('#remote-text').value;
      if (!text && command !== 'read-clipboard') { notify('请先输入要发送的文字', 'error'); return; }
      if (command === 'type') send({type: 2, text});
      else if (command === 'clipboard') send({type: 5, text, paste: false, sequence: sequence++});
      else if (command === 'paste') send({type: 5, text, paste: true, sequence: sequence++});
      else if (command === 'read-clipboard') send({type: 4});
    }
  };

  const configure = options => { notify = options?.toast || notify; installInteractions(); };
  const activate = () => { active = true; connect(); };
  const deactivate = () => {
    if (!active && !videoSocket && !controlSocket) return;
    active = false;
    closeSockets();
    setState('#remote-video-state', '画面未连接');
    setState('#remote-control-state', '控制未连接');
  };

  window.NovaRemoteControl = Object.freeze({configure, activate, deactivate});
})();
