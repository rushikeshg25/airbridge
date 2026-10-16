import './library.js';
import './transfers.js';
import { $, api, bytes, events, recall, remember, showJoin, showView, state, toast } from './common.js';

async function connect() {
  const room = await api('/api/room');
  state.room = room;
  $('room-name').textContent = room.name;
  $('device-name').textContent = room.device;
  $('leave-button').hidden = false;
  $('connection-label').textContent = 'Connected to room';
  $('file-limit').textContent = `Any file type · Up to ${bytes(room.maxFile)} per file`;
  const urls = room.urls.filter((url) => !url.includes('localhost') && !url.includes('127.0.0.1'));
  $('room-url').value = urls[0] || location.origin;
  $('join-dialog').close();
  $('offline-banner').hidden = true;
  events.dispatchEvent(new Event('connected'));
}

$('join-dialog').addEventListener('cancel', (event) => event.preventDefault());
$('join-name').value = recall('name', /Android/i.test(navigator.userAgent) ? 'Android phone' : /iPhone|iPad/i.test(navigator.userAgent) ? 'My iPhone or iPad' : /Mac/i.test(navigator.platform) ? 'My Mac' : 'My computer');
$('join-form').addEventListener('submit', async (event) => {
  event.preventDefault();
  const button = $('join-submit');
  button.disabled = true;
  $('join-error').textContent = '';
  try {
    const name = $('join-name').value.trim();
    await api('/api/join', { method: 'POST', body: JSON.stringify({ name, pin: $('join-pin').value }) });
    remember('name', name);
    $('join-pin').value = '';
    await connect();
  } catch (error) {
    $('join-error').textContent = error.message === 'Failed to fetch' ? 'Cannot reach the host. Check your network and try again.' : error.message;
  } finally { button.disabled = false; }
});
$('leave-button').addEventListener('click', async () => {
  events.dispatchEvent(new Event('leaving'));
  try { await api('/api/leave', { method: 'POST' }); showJoin(); } catch (error) { toast(error.message); }
});
for (const button of document.querySelectorAll('[data-view]')) button.addEventListener('click', () => showView(button.dataset.view));
$('copy-link').addEventListener('click', async () => {
  try {
    if (navigator.clipboard && window.isSecureContext) await navigator.clipboard.writeText($('room-url').value);
    else {
      $('room-url').select();
      if (!document.execCommand('copy')) throw new Error('copy');
      $('room-url').setSelectionRange(0, 0);
    }
    toast('Room address copied. Share the room code separately.');
  } catch { $('room-url').select(); toast('Select and copy the room address above.'); }
});
connect().catch((error) => showJoin(error.status === 401 ? '' : 'Cannot reach the host. Check your connection and try again.'));
