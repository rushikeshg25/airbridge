import { $, api, bytes, element, events, showView, state, toast } from './common.js';

let files = [];
let pending = false;
let signature = '';
function render() {
  const query = $('file-search').value.trim().toLowerCase();
  const visible = files.filter((file) => file.name.toLowerCase().includes(query));
  $('file-count').textContent = files.filter((file) => file.complete).length;
  $('library-summary').textContent = `${visible.length} ${visible.length === 1 ? 'file' : 'files'}`;
  $('library-empty').hidden = files.length > 0 || Boolean(query);
  const next = JSON.stringify([visible, query]);
  if (signature === next) return;
  signature = next;
  const focusID = document.activeElement?.dataset.fileAction;
  $('library-list').replaceChildren();
  for (const file of visible) {
    const row = element('article', 'library-row');
    const icon = element('div', 'file-icon', file.complete ? '▤' : '↗');
    const content = element('div', 'library-content');
    const title = element('strong', 'filename', file.name);
    title.title = file.name;
    const detail = element('p', 'small', `${bytes(file.size)} · ${file.sender} · ${new Date(file.created).toLocaleDateString(undefined, { month: 'short', day: 'numeric' })}`);
    const status = element('span', file.complete ? 'file-status ready' : 'file-status', file.complete ? 'Ready to download' : `${bytes(file.offset)} received · Incomplete`);
    content.append(title, detail, status);
    const actions = element('div', 'file-actions');
    if (file.complete) {
      const download = element('a', 'secondary', 'Download');
      download.href = `/api/files/${file.id}/download`;
      download.download = file.name;
      download.dataset.fileAction = `download-${file.id}`;
      download.setAttribute('aria-label', `Download ${file.name}`);
      actions.append(download);
    } else {
      const resume = element('button', 'text-button', 'Select file to resume');
      resume.dataset.fileAction = `resume-${file.id}`;
      resume.addEventListener('click', () => {
        showView('send');
        toast('On the original sending browser, select the same file to resume.');
        $('file-input').click();
      });
      actions.append(resume);
    }
    const remove = element('button', 'remove-button', '×');
    remove.setAttribute('aria-label', `Remove ${file.name} from the room`);
    remove.title = 'Remove from room';
    remove.dataset.fileAction = `remove-${file.id}`;
    remove.addEventListener('click', async () => {
      if (!confirm(`Remove “${file.name}” from the room? This deletes the host’s stored copy for everyone.`)) return;
      remove.disabled = true;
      try {
        await api(`/api/files/${file.id}`, { method: 'DELETE' });
        events.dispatchEvent(new CustomEvent('file-deleted', { detail: file.id }));
        toast('File removed from the room.'); await refresh();
      } catch (error) { remove.disabled = false; toast(error.message); }
    });
    actions.append(remove); row.append(icon, content, actions); $('library-list').append(row);
  }
  if (!visible.length && query) $('library-list').append(element('p', 'search-empty', 'No files match your search.'));
  if (focusID) document.querySelector(`[data-file-action="${CSS.escape(focusID)}"]`)?.focus({ preventScroll: true });
}

export async function refresh() {
  if (pending || !state.room) return;
  pending = true;
  try {
    const result = await api('/api/files');
    files = result.files;
    $('storage-label').textContent = `${bytes(result.used)} / ${bytes(result.quota)}`;
    $('storage-progress').value = result.quota ? result.used / result.quota * 100 : 0;
    state.online = true;
    $('connection').classList.remove('offline');
    $('offline-banner').hidden = true;
    $('connection-label').textContent = 'Connected to room';
    render();
  } catch (error) {
    if (error.status !== 401) {
      state.online = false;
      $('connection').classList.add('offline');
      $('offline-banner').hidden = false;
      $('connection-label').textContent = 'Host unreachable';
    }
  } finally { pending = false; }
}
$('refresh-library').addEventListener('click', async () => { await refresh(); toast(state.online ? 'Room files refreshed.' : 'Host is unreachable. Check your connection.'); });
$('file-search').addEventListener('input', render);
events.addEventListener('connected', refresh);
events.addEventListener('files-changed', refresh);
events.addEventListener('view', (event) => { if (event.detail === 'library') void refresh(); });
setInterval(() => { if (!document.hidden) void refresh(); }, 3000);
document.addEventListener('visibilitychange', () => { if (!document.hidden) void refresh(); });
