(() => {
  'use strict';
  const output = document.querySelector('#output');
  const input = document.querySelector('#input');
  const state = document.querySelector('#state');
  const encoder = new TextEncoder();
  const decoder = new TextDecoder();
  const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:';
  const socket = new WebSocket(`${protocol}//${location.host}/term`);
  socket.binaryType = 'arraybuffer';

  const setState = (label, className) => {
    state.lastElementChild.textContent = label;
    state.className = `terminal-state ${className}`;
  };
  const send = value => {
    if (socket.readyState !== WebSocket.OPEN) return false;
    socket.send(encoder.encode(`\0${value}`));
    return true;
  };
  const resize = () => {
    if (socket.readyState !== WebSocket.OPEN) return;
    const cols = Math.max(20, Math.floor(output.clientWidth / 8));
    const rows = Math.max(8, Math.floor(output.clientHeight / 17));
    const body = encoder.encode(JSON.stringify({cols, rows}));
    const message = new Uint8Array(body.length + 1);
    message[0] = 1;
    message.set(body, 1);
    socket.send(message);
  };

  socket.addEventListener('open', () => { setState('已连接', 'connected'); resize(); });
  socket.addEventListener('message', event => {
    const text = typeof event.data === 'string' ? event.data : decoder.decode(new Uint8Array(event.data), {stream: true});
    output.textContent += text.replace(/\x1b][^\x07]*(?:\x07|\x1b\\)/g, '').replace(/\x1b\[[0-?]*[ -/]*[@-~]/g, '');
    output.scrollTop = output.scrollHeight;
  });
  socket.addEventListener('close', () => setState('会话已结束', 'failed'));
  socket.addEventListener('error', () => setState('连接失败，请重新认证', 'failed'));
  document.querySelector('#command').addEventListener('submit', event => {
    event.preventDefault();
    if (!input.value || !send(`${input.value}\r`)) return;
    input.value = '';
  });
  document.querySelector('#interrupt').addEventListener('click', () => send('\x03'));
  window.addEventListener('resize', resize);
})();
