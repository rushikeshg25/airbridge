export const $ = (id) => document.getElementById(id);
export const events = new EventTarget();
export const state = { room: null, online: true, generation: 0 };
export function bytes(value) {
  if (!Number.isFinite(value) || value <= 0) return '0 B';
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB'];
  const index = Math.min(Math.floor(Math.log(value) / Math.log(1024)), units.length - 1);
  return `${(value / 1024 ** index).toFixed(index > 0 && value / 1024 ** index < 10 ? 1 : 0)} ${units[index]}`;
}
export function element(tag, className, text) {
  const node = document.createElement(tag);
  if (className) node.className = className;
  if (text !== undefined) node.textContent = text;
  return node;
}
let toastTimer;
export function toast(message) {
  $('toast').textContent = message;
  $('toast').hidden = false;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => { $('toast').hidden = true; }, 4500);
}
export function showJoin(message = '') {
  state.generation++;
  state.room = null;
  events.dispatchEvent(new Event('leaving'));
  $('join-error').textContent = message;
  $('connection-label').textContent = 'Not connected';
  $('leave-button').hidden = true;
  if (!$('join-dialog').open) $('join-dialog').showModal();
}
export async function api(path, options = {}) {
  const generation = state.generation;
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), path.endsWith('/complete') ? 120000 : 30000);
  let response, body;
  try {
    response = await fetch(path, {
      ...options,
      signal: controller.signal,
      headers: { 'Content-Type': 'application/json', ...options.headers },
    });
    body = await response.json();
  } catch (error) {
    throw new Error(error.name === 'AbortError'
      ? 'The host took too long to respond. Check the connection and retry.'
      : 'Cannot reach the host. Check your network and retry.');
  } finally { clearTimeout(timer); }
  if (!response.ok) {
    if (response.status === 401 && path !== '/api/join' && generation === state.generation) showJoin(body.error);
    const error = new Error(body.error || `Request failed (${response.status})`);
    error.status = response.status;
    throw error;
  }
  return body;
}
export function showView(name) {
  if (!['send', 'library', 'help'].includes(name)) return;
  for (const view of document.querySelectorAll('.view')) view.hidden = view.id !== `view-${name}`;
  for (const button of document.querySelectorAll('.nav-item')) {
    const active = button.dataset.view === name;
    button.classList.toggle('active', active);
    if (active) button.setAttribute('aria-current', 'page');
    else button.removeAttribute('aria-current');
  }
  $('page-label').textContent = { send: 'Send files', library: 'Room files', help: 'How it works' }[name];
  events.dispatchEvent(new CustomEvent('view', { detail: name }));
}
export function remember(key, value) {
  try { localStorage.setItem(`airbridge:${key}`, JSON.stringify(value)); } catch { /* Private browsing or a full storage area must not stop a transfer. */ }
}
export function recall(key, fallback) {
  try { return JSON.parse(localStorage.getItem(`airbridge:${key}`)) ?? fallback; } catch { return fallback; }
}
