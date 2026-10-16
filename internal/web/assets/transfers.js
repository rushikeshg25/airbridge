import { $, api, bytes, element, events, recall, remember, showJoin, state, toast } from './common.js';

const queue = [];
let active = 0;
const MAX_ACTIVE = 2;
let saved = recall('uploads', {});
if (!saved || typeof saved !== 'object' || Array.isArray(saved)) saved = {};
function checkpoint(item) {
  if (item.status === 'done') delete saved[item.key];
  else if (item.id) saved[item.key] = item.id;
  const keys = Object.keys(saved);
  for (const key of keys.slice(0, Math.max(0, keys.length - 500))) delete saved[key];
  remember('uploads', saved);
}
function pause(item) {
  item.paused = true;
  if (item.xhr) item.xhr.abort();
  if (!item.running) item.status = 'paused';
  draw(item);
}
function resume(item) {
  if (item.running) return;
  item.paused = false; item.status = 'queued'; item.error = ''; draw(item); pump();
}

function draw(item) {
  if (!item.ui) {
    const card = element('article', 'transfer-row');
    const icon = element('div', 'file-icon', '↗');
    const content = element('div', 'transfer-content');
    const title = element('strong', 'filename', item.file.name);
    title.title = item.file.name;
    const info = element('p', 'transfer-info');
    const progress = document.createElement('progress');
    progress.max = item.file.size || 1;
    progress.setAttribute('aria-label', `Upload progress for ${item.file.name}`);
    content.append(title, info, progress);
    const actions = element('div', 'transfer-actions');
    const retry = element('button', 'text-button', 'Retry');
    retry.addEventListener('click', () => resume(item));
    const pauseButton = element('button', 'text-button', 'Pause');
    pauseButton.addEventListener('click', () => pause(item));
    const resumeButton = element('button', 'text-button', 'Resume');
    resumeButton.addEventListener('click', () => resume(item));
    actions.append(pauseButton, resumeButton, retry);
    card.append(icon, content, actions);
    $('transfer-list').append(card);
    item.ui = { card, info, progress, retry, actions, icon, pauseButton, resumeButton };
  }
  const labels = { queued: 'Waiting', sending: 'Sending', done: 'Shared with room', error: 'Needs attention', paused: 'Paused' };
  item.ui.card.dataset.status = item.status;
  item.ui.icon.textContent = item.status === 'done' ? '✓' : '↗';
  item.ui.progress.value = item.status === 'done' ? item.file.size || 1 : item.visible || 0;
  item.ui.info.textContent = item.error || `${labels[item.status]} · ${bytes(item.visible || 0)} / ${bytes(item.file.size)}${item.speed && item.status === 'sending' ? ` · ${bytes(item.speed)}/s · ~${Math.max(1, Math.ceil((item.file.size - (item.visible || 0)) / item.speed))}s left` : ''}`;
  item.ui.retry.hidden = item.status !== 'error';
  item.ui.pauseButton.hidden = !['sending', 'queued'].includes(item.status);
  item.ui.pauseButton.disabled = Boolean(item.paused);
  item.ui.resumeButton.hidden = item.status !== 'paused';
  item.ui.resumeButton.disabled = Boolean(item.running);
  $('queue-empty').hidden = queue.length > 0;
  $('queue-count').textContent = queue.length;
  $('clear-complete').hidden = !queue.some((item) => item.status === 'done');
}

function sendChunk(item, offset, end) {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    item.xhr = xhr;
    xhr.open('PATCH', `/api/files/${item.id}`);
    xhr.setRequestHeader('Upload-Offset', String(offset));
    xhr.setRequestHeader('Content-Type', 'application/octet-stream');
    xhr.timeout = 125000;
    const start = performance.now();
    xhr.upload.onprogress = (event) => {
      item.visible = offset + event.loaded;
      item.speed = event.loaded / Math.max((performance.now() - start) / 1000, 0.1);
      draw(item);
    };
    xhr.onload = () => {
      item.xhr = null;
      let response;
      try { response = JSON.parse(xhr.responseText); } catch { reject(new Error('The host returned an invalid response.')); return; }
      if (xhr.status >= 200 && xhr.status < 300) resolve(response);
      else { const error = new Error(response.error || 'Upload failed.'); error.status = xhr.status; reject(error); }
    };
    xhr.onerror = xhr.ontimeout = () => { item.xhr = null; reject(new Error('Connection interrupted. Retry to resume.')); };
    xhr.onabort = () => { item.xhr = null; reject(new DOMException('Upload paused', 'AbortError')); };
    xhr.send(item.file.slice(offset, end));
  });
}

async function run(item) {
  item.running = true;
  item.status = 'sending';
  item.error = '';
  draw(item);
  try {
    let remote;
    if (item.id) {
      try { remote = await api(`/api/files/${item.id}`); }
      catch (error) { if (error.status !== 404) throw error; item.id = null; }
    }
    if (!item.id) {
      remote = await api('/api/files', { method: 'POST', body: JSON.stringify({ name: item.file.name, size: item.file.size }) });
      item.id = remote.id;
      checkpoint(item);
    }
    item.visible = remote.offset;
    if (item.paused) { item.status = 'paused'; return; }
    let retries = 0;
    while (remote.offset < item.file.size) {
      if (item.paused) { item.status = 'paused'; return; }
      try {
        remote = await sendChunk(item, remote.offset, Math.min(remote.offset + state.room.chunkSize, item.file.size));
        retries = 0;
      } catch (error) {
        if (item.paused || error.name === 'AbortError') throw error;
        if (error.status === 401) { showJoin(error.message); throw error; }
        if ((error.status && error.status !== 409 && error.status < 500) || retries >= 3) throw error;
        item.error = `Connection interrupted. Reconnecting (${++retries}/3)…`; draw(item);
        await new Promise((resolve) => setTimeout(resolve, 1000 * 2 ** (retries - 1)));
        if (item.paused) { item.status = 'paused'; return; }
        remote = await api(`/api/files/${item.id}`);
        item.error = '';
      }
      item.visible = remote.offset;
    }
    if (item.paused) { item.status = 'paused'; return; }
    await api(`/api/files/${item.id}/complete`, { method: 'POST' });
    item.status = 'done';
    checkpoint(item);
    item.speed = 0;
    item.visible = item.file.size;
    events.dispatchEvent(new Event('files-changed'));
    $('transfer-announcement').textContent = `${item.file.name} is ready in Room files.`;
  } catch (error) {
    item.status = item.paused ? 'paused' : 'error';
    item.error = item.paused ? '' : error.message;
    if (item.paused && item.id && state.room) {
      try { item.visible = (await api(`/api/files/${item.id}`)).offset; } catch { /* Resume will reconcile the acknowledged offset. */ }
    }
  } finally {
    item.running = false; item.speed = 0; active--; draw(item); pump();
  }
}

function pump() {
  if (!state.room) return;
  for (const item of queue) {
    if (active >= MAX_ACTIVE) break;
    if (item.status === 'queued') { active++; void run(item); }
  }
}
async function enqueue(files) {
  const room = state.room;
  if (!room) { toast('Join the room first.'); return; }
  for (const file of files) {
    if (state.room !== room) return;
    if (file.size > room.maxFile) { toast(`${file.name} exceeds the ${bytes(room.maxFile)} file limit.`); continue; }
    // Hash only two 64 KiB samples on the host. This works on HTTP LAN
    // origins where browser WebCrypto is unavailable and avoids a full-file scan.
    let identity;
    try {
      const sample = new Blob([file.slice(0, 65536), file.slice(Math.max(0, file.size - 65536))]);
      identity = await api('/api/fingerprint', { method: 'POST', body: sample, headers: { 'Content-Type': 'application/octet-stream' } });
    } catch (error) { toast(error.message); continue; }
    if (state.room !== room) return;
    const key = JSON.stringify([file.name, file.size, file.lastModified, identity.hash]);
    if (queue.some((item) => item.key === key && item.status !== 'done')) { toast(`${file.name} is already in your queue.`); continue; }
    const item = { file, key, id: saved[key] || null, status: 'queued', visible: 0, speed: 0, paused: false };
    queue.push(item); draw(item); pump();
  }
  pump();
}
$('choose-button').addEventListener('click', () => $('file-input').click());
$('file-input').addEventListener('change', () => { void enqueue(Array.from($('file-input').files)); $('file-input').value = ''; });
for (const type of ['dragenter', 'dragover']) $('dropzone').addEventListener(type, (event) => { event.preventDefault(); $('dropzone').classList.add('dragging'); });
$('dropzone').addEventListener('dragleave', () => $('dropzone').classList.remove('dragging'));
$('dropzone').addEventListener('drop', (event) => { event.preventDefault(); $('dropzone').classList.remove('dragging'); void enqueue(Array.from(event.dataTransfer.files)); });
// Prevent an accidental drop outside the target from replacing this page.
window.addEventListener('dragover', (event) => event.preventDefault());
window.addEventListener('drop', (event) => event.preventDefault());
$('clear-complete').addEventListener('click', () => {
  for (let i = queue.length - 1; i >= 0; i--) if (queue[i].status === 'done') { queue[i].ui.card.remove(); queue.splice(i, 1); }
  $('queue-count').textContent = queue.length;
  $('queue-empty').hidden = queue.length > 0;
  $('clear-complete').hidden = true;
});
events.addEventListener('connected', pump);
events.addEventListener('leaving', () => { for (const item of queue) if (['sending', 'queued'].includes(item.status)) pause(item); });
window.addEventListener('beforeunload', (event) => {
  if (queue.some((item) => item.status === 'sending' || item.status === 'queued')) { event.preventDefault(); event.returnValue = ''; }
});

events.addEventListener('file-deleted', (event) => {
  for (const item of queue) if (item.id === event.detail) {
    pause(item);
    delete saved[item.key]; remember('uploads', saved);
    item.id = null;
    item.error = 'Removed from the room. Resume to send a new copy.';
    if (item.status === 'done') item.status = 'paused';
    draw(item);
  }
});
