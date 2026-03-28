// === State ===
let findings = [];
let selectedIndex = -1;
let activeFilter = 'all';
let chatOpen = true;
let sidebarOpen = true;
let polling = null;

// === Init ===
document.addEventListener('DOMContentLoaded', () => {
  loadFindings();
  setupFilters();
  updateToggleButtons();
  setupKeyboardShortcuts();
});

// === API ===
async function api(path, opts = {}) {
  const resp = await fetch(path, {
    headers: { 'Content-Type': 'application/json' },
    ...opts,
  });
  return resp.json();
}

// === Data Loading ===
async function loadFindings() {
  const data = await api('/api/findings');
  const prevSelected = selectedIndex;
  findings = data;
  renderSidebar();
  renderSummary();
  if (prevSelected >= 0 && prevSelected < findings.length) {
    // Re-render detail if something changed (e.g., fix completed)
    renderDetail(prevSelected);
  }
}

function renderSummary() {
  const el = document.getElementById('summary');
  let errors = 0, warns = 0, infos = 0, fixed = 0;
  for (const f of findings) {
    if (f.action === 'fixed') fixed++;
    switch (f.finding.severity) {
      case 2: errors++; break;
      case 1: warns++; break;
      default: infos++; break;
    }
  }
  el.innerHTML = `
    <span class="count count-error">${errors} errors</span>
    <span class="count count-warn">${warns} warnings</span>
    <span class="count count-info">${infos} info</span>
    <span class="count count-fixed">${fixed} fixed</span>
  `;
}

// === Sidebar ===
function setupFilters() {
  document.querySelectorAll('.filter-btn').forEach(btn => {
    btn.addEventListener('click', () => {
      document.querySelectorAll('.filter-btn').forEach(b => b.classList.remove('active'));
      btn.classList.add('active');
      activeFilter = btn.dataset.filter;
      renderSidebar();
    });
  });
}

function renderSidebar() {
  const list = document.getElementById('findings-list');
  list.innerHTML = '';

  findings.forEach((item, index) => {
    if (!matchesFilter(item)) return;

    const f = item.finding;
    const el = document.createElement('div');
    el.className = 'finding-item' +
      (index === selectedIndex ? ' active' : '') +
      (item.action === 'dismissed' ? ' dismissed' : '');
    el.onclick = () => selectFinding(index);

    const sevClass = ['info', 'warn', 'error'][f.severity] || 'info';
    const statusHTML = item.action !== 'pending'
      ? `<span class="fi-status fi-status-${item.action}">${item.action}</span>`
      : '';

    el.innerHTML = `
      <div class="sev sev-${sevClass}"></div>
      <div class="fi-body">
        <div class="fi-location">${escapeHTML(formatLocation(f))}</div>
        <div class="fi-message">${escapeHTML(f.message)}</div>
      </div>
      ${statusHTML}
    `;
    list.appendChild(el);
  });
}

function matchesFilter(item) {
  if (activeFilter === 'all') return true;
  if (activeFilter === 'pending') return item.action === 'pending';
  if (activeFilter === 'error') return item.finding.severity === 2;
  if (activeFilter === 'warn') return item.finding.severity === 1;
  return true;
}

// === Finding Detail ===
function selectFinding(index) {
  selectedIndex = index;
  renderSidebar();
  renderDetail(index);
  loadDiff(index);
}

function renderDetail(index) {
  const item = findings[index];
  if (!item) return;
  const f = item.finding;

  document.getElementById('empty-state').style.display = 'none';
  document.getElementById('detail').style.display = 'block';

  // Severity badge
  const sevEl = document.getElementById('detail-severity');
  const sevName = ['info', 'warn', 'error'][f.severity] || 'info';
  sevEl.className = `severity-badge ${sevName}`;
  sevEl.textContent = sevName.toUpperCase();

  // Location & source
  document.getElementById('detail-location').textContent = formatLocation(f);
  document.getElementById('detail-source').textContent = f.source === 'agent' ? 'AI Agent' : f.rule_id;

  // Action badge
  const actionEl = document.getElementById('detail-action');
  if (item.action !== 'pending') {
    actionEl.className = `action-badge show ${item.action}`;
    actionEl.textContent = item.action;
  } else {
    actionEl.className = 'action-badge';
  }

  // Message
  document.getElementById('detail-message').textContent = f.message;

  // Suggestion
  const sugEl = document.getElementById('detail-suggestion');
  if (f.suggestion) {
    sugEl.style.display = 'block';
    document.getElementById('suggestion-text').textContent = f.suggestion;
  } else {
    sugEl.style.display = 'none';
  }

  // Meta
  const metaParts = [];
  if (f.category) metaParts.push(`Category: ${f.category}`);
  if (f.confidence > 0) metaParts.push(`Confidence: ${['low', 'medium', 'high'][f.confidence]}`);
  document.getElementById('detail-meta').textContent = metaParts.join(' | ');

  // Proposal panel
  const proposalPanel = document.getElementById('proposal-panel');
  if (item.proposal) {
    proposalPanel.style.display = 'block';
    document.getElementById('proposal-explanation').textContent = item.proposal.explanation;
    renderProposalDiff(item.proposal.original, item.proposal.fixed);
  } else {
    proposalPanel.style.display = 'none';
  }

  // Fix error
  if (item.fix_error) {
    const metaEl = document.getElementById('detail-meta');
    metaEl.innerHTML += `<div style="color:var(--accent-red);margin-top:8px">Fix error: ${escapeHTML(item.fix_error)}</div>`;
  }

  // Explanation
  if (item.explanation && item.action === 'fixed') {
    const metaEl = document.getElementById('detail-meta');
    metaEl.innerHTML += `<div style="color:var(--accent-green);margin-top:8px">Fix applied: ${escapeHTML(item.explanation)}</div>`;
  }
}

async function loadDiff(index) {
  const data = await api(`/api/diff/${index}`);
  const container = document.getElementById('diff-view');
  container.innerHTML = '';

  if (!data.lines || data.lines.length === 0) {
    container.innerHTML = '<div class="diff-line" style="color:var(--text-muted);padding:16px">No diff context available</div>';
    return;
  }

  for (const line of data.lines) {
    const el = document.createElement('div');
    el.className = 'diff-line ' + line.type + (line.highlight ? ' highlight' : '');

    if (line.type === 'header') {
      el.textContent = line.content;
    } else {
      const prefix = line.type === 'added' ? '+' : line.type === 'removed' ? '-' : ' ';
      const numStr = line.num > 0 ? String(line.num) : '';
      el.innerHTML = `<span class="line-num">${numStr}</span><span class="line-prefix">${prefix}</span>${escapeHTML(line.content)}`;
    }
    container.appendChild(el);
  }
}

// === Proposal Diff ===
function renderProposalDiff(original, fixed) {
  const container = document.getElementById('proposal-diff');
  container.innerHTML = '';

  const origLines = original.split('\n');
  const fixedLines = fixed.split('\n');

  // Simple diff: find changed region
  let start = 0;
  const minLen = Math.min(origLines.length, fixedLines.length);
  while (start < minLen && origLines[start] === fixedLines[start]) start++;

  let endOrig = origLines.length - 1;
  let endFixed = fixedLines.length - 1;
  while (endOrig > start && endFixed > start && origLines[endOrig] === fixedLines[endFixed]) {
    endOrig--;
    endFixed--;
  }

  // Context before
  const ctxStart = Math.max(0, start - 3);
  for (let i = ctxStart; i < start; i++) {
    addDiffLine(container, 'context', origLines[i], i + 1);
  }

  // Removed
  for (let i = start; i <= endOrig && i < origLines.length; i++) {
    addDiffLine(container, 'removed', origLines[i], i + 1);
  }

  // Added
  for (let i = start; i <= endFixed && i < fixedLines.length; i++) {
    addDiffLine(container, 'added', fixedLines[i], i + 1);
  }

  // Context after
  const ctxEnd = Math.min(origLines.length - 1, endOrig + 3);
  for (let i = endOrig + 1; i <= ctxEnd; i++) {
    addDiffLine(container, 'context', origLines[i], i + 1);
  }

  if (container.children.length === 0) {
    container.innerHTML = '<div class="diff-line" style="color:var(--text-muted);padding:16px">No changes</div>';
  }
}

function addDiffLine(container, type, content, num) {
  const el = document.createElement('div');
  el.className = 'diff-line ' + type;
  const prefix = type === 'added' ? '+' : type === 'removed' ? '-' : ' ';
  el.innerHTML = `<span class="line-num">${num || ''}</span><span class="line-prefix">${prefix}</span>${escapeHTML(content)}`;
  container.appendChild(el);
}

// === Actions ===
async function doAction(action) {
  if (selectedIndex < 0) return;

  if (action === 'fix') {
    // Request AI fix proposal
    const resp = await api(`/api/findings/${selectedIndex}/fix`, { method: 'POST' });
    if (resp.status === 'fixing') {
      findings[selectedIndex].action = 'fixing';
      renderSidebar();
      renderDetail(selectedIndex);
      // Poll until fix completes
      startFixPolling();
    }
    return;
  }

  await api(`/api/findings/${selectedIndex}/${action}`, { method: 'POST' });
  await loadFindings();
  if (selectedIndex >= 0) renderDetail(selectedIndex);
}

function startFixPolling() {
  if (polling) return; // already polling
  polling = setInterval(async () => {
    await loadFindings();
    if (selectedIndex >= 0) renderDetail(selectedIndex);
    // Stop polling when no findings are in 'fixing' state
    const anyFixing = findings.some(f => f.action === 'fixing');
    if (!anyFixing) {
      clearInterval(polling);
      polling = null;
    }
  }, 1500);
}

async function applyFix() {
  if (selectedIndex < 0) return;
  await api(`/api/findings/${selectedIndex}/fix-apply`, { method: 'POST' });
  await loadFindings();
  if (selectedIndex >= 0) renderDetail(selectedIndex);
}

async function cancelFix() {
  if (selectedIndex < 0) return;
  await api(`/api/findings/${selectedIndex}/fix-cancel`, { method: 'POST' });
  await loadFindings();
  if (selectedIndex >= 0) renderDetail(selectedIndex);
}

// === Chat ===
let chatSending = false;

function toggleChat() {
  chatOpen = !chatOpen;
  const panel = document.getElementById('chat-panel');
  if (chatOpen) {
    panel.classList.remove('collapsed');
  } else {
    panel.classList.add('collapsed');
  }
  updateToggleButtons();
}

function toggleSidebar() {
  sidebarOpen = !sidebarOpen;
  const sidebar = document.getElementById('sidebar');
  if (sidebarOpen) {
    sidebar.classList.remove('collapsed');
  } else {
    sidebar.classList.add('collapsed');
  }
  updateToggleButtons();
}

function updateToggleButtons() {
  const sidebarBtn = document.getElementById('sidebar-toggle');
  const chatBtn = document.getElementById('chat-panel-toggle');
  if (sidebarBtn) sidebarBtn.classList.toggle('active', sidebarOpen);
  if (chatBtn) chatBtn.classList.toggle('active', chatOpen);
}

async function sendChat() {
  if (selectedIndex < 0) {
    alert('Please select a finding first');
    return;
  }
  if (chatSending) return;

  const input = document.getElementById('chat-input');
  const message = input.value.trim();
  if (!message) return;

  input.value = '';
  addChatMessage('user', message);

  // Check if this is a fix confirmation
  const lower = message.toLowerCase();
  if (findings[selectedIndex].proposal) {
    if (['accept', 'apply', 'yes', 'y', 'ok'].includes(lower)) {
      await applyFix();
      addChatMessage('system', 'Fix applied.');
      await loadFindings();
      return;
    } else if (['cancel', 'reject', 'no', 'n'].includes(lower)) {
      await cancelFix();
      addChatMessage('system', 'Fix discarded.');
      await loadFindings();
      return;
    }
  }

  chatSending = true;

  // Create assistant message element for streaming
  const msgEl = addChatMessage('assistant', '');
  let fullText = '';

  try {
    // Use AbortController so we can cancel if needed, but NOT on panel toggle
    const controller = new AbortController();
    const resp = await fetch('/api/chat/stream', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ index: selectedIndex, message }),
      signal: controller.signal,
    });

    const reader = resp.body.getReader();
    const decoder = new TextDecoder();
    let buffer = '';

    while (true) {
      const { done, value } = await reader.read();
      if (done) break;

      buffer += decoder.decode(value, { stream: true });

      // Parse SSE events from buffer
      const lines = buffer.split('\n');
      buffer = lines.pop() || ''; // keep incomplete line in buffer

      for (const line of lines) {
        if (!line.startsWith('data: ')) continue;
        const jsonStr = line.slice(6);
        if (!jsonStr) continue;

        try {
          const data = JSON.parse(jsonStr);

          if (data.error) {
            msgEl.remove();
            addChatMessage('system', 'Error: ' + data.error);
            break;
          }

          if (data.token) {
            // Streaming token — update even if panel is collapsed
            fullText += data.token;
            renderMarkdown(msgEl, fullText);
            // Only scroll if chat panel is visible
            if (chatOpen) scrollChatToBottom();
          }

          if (data.done) {
            // Final render — immediate, full text is complete
            renderMarkdown(msgEl, fullText, true);
            if (chatOpen) scrollChatToBottom();
          }
        } catch (e) {
          // Skip malformed JSON
        }
      }
    }
  } catch (err) {
    // Don't show error for AbortError (user cancelled)
    if (err.name !== 'AbortError') {
      if (!fullText) {
        msgEl.remove();
      }
      addChatMessage('system', 'Error: ' + err.message);
    }
  }

  // Final scroll when stream ends (in case panel was collapsed during stream)
  if (chatOpen) scrollChatToBottom();
  chatSending = false;
}

function addChatMessage(role, content) {
  const container = document.getElementById('chat-messages');

  // Remove placeholder
  const placeholder = container.querySelector('.chat-placeholder');
  if (placeholder) placeholder.remove();

  const el = document.createElement('div');
  el.className = `chat-msg ${role}`;

  if (role === 'assistant' && content) {
    renderMarkdown(el, content, true);
  } else {
    el.textContent = content;
  }

  container.appendChild(el);
  scrollChatToBottom();
  return el;
}

// Render markdown into an element. During streaming, we throttle renders
// to avoid broken HTML from partial markdown text.
let _renderTimer = null;
function renderMarkdown(el, text, immediate) {
  if (typeof marked === 'undefined' || typeof marked.parse !== 'function') {
    // marked not loaded — show as plain text
    el.innerText = text;
    return;
  }

  if (immediate) {
    _doRender(el, text);
    return;
  }

  // Throttle: render at most every 100ms during streaming to avoid
  // broken HTML from incomplete markdown (e.g., unclosed code blocks)
  el._pendingText = text;
  if (!_renderTimer) {
    _renderTimer = setTimeout(() => {
      _renderTimer = null;
      if (el._pendingText != null) {
        _doRender(el, el._pendingText);
      }
    }, 100);
  }
}

function _doRender(el, text) {
  try {
    const html = marked.parse(text);
    el.innerHTML = html;
  } catch (e) {
    console.warn('[cr] marked.parse failed:', e);
    el.innerText = text;
  }
}

function scrollChatToBottom() {
  const container = document.getElementById('chat-messages');
  container.scrollTop = container.scrollHeight;
}

// === Keyboard Shortcuts ===
function setupKeyboardShortcuts() {
  const chatInput = document.getElementById('chat-input');
  const overlay = document.getElementById('shortcuts-overlay');

  document.addEventListener('keydown', (e) => {
    // Toggle help overlay with '?'
    if (e.key === '?' && document.activeElement !== chatInput) {
      e.preventDefault();
      toggleShortcutsOverlay();
      return;
    }

    // Close overlay on Escape (regardless of focus)
    if (e.key === 'Escape') {
      if (overlay && !overlay.classList.contains('hidden')) {
        overlay.classList.add('hidden');
        return;
      }
      if (document.activeElement === chatInput) {
        chatInput.blur();
        return;
      }
      return;
    }

    // Don't capture keys when chat input is focused
    if (document.activeElement === chatInput) return;

    // Don't capture when modifier keys are held (allow browser shortcuts)
    if (e.ctrlKey || e.metaKey || e.altKey) return;

    switch (e.key) {
      case 'j':
      case 'ArrowDown':
        e.preventDefault();
        navigateFinding(1);
        break;
      case 'k':
      case 'ArrowUp':
        e.preventDefault();
        navigateFinding(-1);
        break;
      case 'a':
        e.preventDefault();
        doAction('accept');
        break;
      case 'd':
        e.preventDefault();
        doAction('dismiss');
        break;
      case 'f':
        e.preventDefault();
        doAction('fix');
        break;
      case 'r':
        e.preventDefault();
        doAction('reset');
        break;
      case 'Tab':
        e.preventDefault();
        jumpToNextPending();
        break;
      case '/':
      case 'c':
        e.preventDefault();
        if (chatInput) {
          if (!chatOpen) toggleChat();
          chatInput.focus();
        }
        break;
    }
  });
}

function navigateFinding(direction) {
  if (findings.length === 0) return;
  // Collect visible indices (those matching the current filter)
  const visibleIndices = [];
  findings.forEach((item, index) => {
    if (matchesFilter(item)) visibleIndices.push(index);
  });
  if (visibleIndices.length === 0) return;

  const currentPos = visibleIndices.indexOf(selectedIndex);
  let nextPos;
  if (currentPos < 0) {
    nextPos = direction > 0 ? 0 : visibleIndices.length - 1;
  } else {
    nextPos = currentPos + direction;
    if (nextPos < 0) nextPos = 0;
    if (nextPos >= visibleIndices.length) nextPos = visibleIndices.length - 1;
  }
  selectFinding(visibleIndices[nextPos]);
}

function jumpToNextPending() {
  if (findings.length === 0) return;
  const start = selectedIndex < 0 ? 0 : selectedIndex + 1;
  for (let i = 0; i < findings.length; i++) {
    const idx = (start + i) % findings.length;
    if (findings[idx].action === 'pending') {
      selectFinding(idx);
      return;
    }
  }
}

function toggleShortcutsOverlay() {
  const overlay = document.getElementById('shortcuts-overlay');
  if (!overlay) return;
  overlay.classList.toggle('hidden');
}

// === Helpers ===
function formatLocation(f) {
  if (!f.line || f.line === 0) return f.file_path;
  if (f.end_line > 0 && f.end_line !== f.line) return `${f.file_path}:${f.line}-${f.end_line}`;
  return `${f.file_path}:${f.line}`;
}

function escapeHTML(str) {
  if (!str) return '';
  return str.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}
