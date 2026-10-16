import { $, api, bytes, element, events, state, toast } from './common.js';

const queue = [];
let active = 0;
const MAX_ACTIVE = 2;

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
    retry.addEventListener('click', () => { item.status = 'queued'; item.error = ''; draw(item); pump(); });
    actions.append(retry);
    card.append(icon, content, actions);
    $('transfer-list').append(card);
    item.ui = { card, info, progress, retry, actions, icon };
  }
  const labels = { queued: 'Waiting', sending: 'Sending', done: 'Shared with room', error: 'Needs attention', paused: 'Paused' };
  item.ui.card.dataset.status = item.status;
  item.ui.icon.textContent = item.status === 'done' ? '✓' : '↗';
  item.ui.progress.value = item.status === 'done' ? item.file.size || 1 : item.visible || 0;
  item.ui.info.textContent = item.error || `${labels[item.status]} · ${bytes(item.visible || 0)} / ${bytes(item.file.size)}${item.speed ? ` · ${bytes(item.speed)}/s` : ''}`;
  item.ui.retry.hidden = item.status !== 'error';
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
  item.status = 'sending';
  item.error = '';
  draw(item);
  try {
    let remote;
    if (item.id) remote = await api(`/api/files/${item.id}`);
    else {
      remote = await api('/api/files', { method: 'POST', body: JSON.stringify({ name: item.file.name, size: item.file.size }) });
      item.id = remote.id;
    }
    item.visible = remote.offset;
    while (remote.offset < item.file.size) {
      remote = await sendChunk(item, remote.offset, Math.min(remote.offset + state.room.chunkSize, item.file.size));
      item.visible = remote.offset;
    }
    await api(`/api/files/${item.id}/complete`, { method: 'POST' });
    item.status = 'done';
    item.speed = 0;
    item.visible = item.file.size;
    events.dispatchEvent(new Event('files-changed'));
  } catch (error) { item.status = 'error'; item.error = error.message; item.speed = 0; }
  finally { active--; draw(item); pump(); }
}

function pump() {
  if (!state.room) return;
  for (const item of queue) {
    if (active >= MAX_ACTIVE) break;
    if (item.status === 'queued') { active++; void run(item); }
  }
}
function enqueue(files) {
  if (!state.room) { toast('Join the room first.'); return; }
  for (const file of files) {
    if (file.size > state.room.maxFile) { toast(`${file.name} exceeds the ${bytes(state.room.maxFile)} file limit.`); continue; }
    const item = { file, id: null, status: 'queued', visible: 0, speed: 0 };
    queue.push(item); draw(item);
  }
  pump();
}
$('choose-button').addEventListener('click', () => $('file-input').click());
$('file-input').addEventListener('change', () => { enqueue($('file-input').files); $('file-input').value = ''; });
for (const type of ['dragenter', 'dragover']) $('dropzone').addEventListener(type, (event) => { event.preventDefault(); $('dropzone').classList.add('dragging'); });
$('dropzone').addEventListener('dragleave', () => $('dropzone').classList.remove('dragging'));
$('dropzone').addEventListener('drop', (event) => { event.preventDefault(); $('dropzone').classList.remove('dragging'); enqueue(event.dataTransfer.files); });
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
window.addEventListener('beforeunload', (event) => {
  if (queue.some((item) => item.status === 'sending' || item.status === 'queued')) { event.preventDefault(); event.returnValue = ''; }
});
