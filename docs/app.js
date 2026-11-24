/**
 * ZTools Showcase Web Application
 * Dynamic Tool Catalog & Registry Explorer
 */

// SVG Icons Map
const ICONS = {
  android: `<svg viewBox="0 0 24 24" fill="currentColor"><path d="M17.523 15.3414c-.5511 0-.9993-.4486-.9993-1.0001s.4482-.9993.9993-.9993c.551 0 .9992.4478.9992.9993 0 .5515-.4482 1.0001-.9992 1.0001m-11.046 0c-.5511 0-.9993-.4486-.9993-1.0001s.4482-.9993.9993-.9993c.5511 0 .9993.4478.9993.9993 0 .5515-.4482 1.0001-.9993 1.0001m11.4045-6.02l1.9973-3.4592a.416.416 0 0 0-.1521-.5676.416.416 0 0 0-.5676.1521l-2.0223 3.503C15.5902 8.4116 13.8533 8.08 12 8.08s-3.5902.3316-5.1368.8697L4.8409 5.4467a.4161.4161 0 0 0-.5677-.1521.4157.4157 0 0 0-.1521.5676l1.9973 3.4592C2.6889 11.1867.3432 14.6589 0 18.761h24c-.3432-4.1021-2.6889-7.5743-6.1185-9.4396"/></svg>`,
  search: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="11" cy="11" r="8"></circle><line x1="21" y1="21" x2="16.65" y2="16.65"></line></svg>`,
  globe: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"></circle><line x1="2" y1="12" x2="22" y2="12"></line><path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"></path></svg>`,
  chrome: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"></circle><circle cx="12" cy="12" r="4"></circle><line x1="21.17" y1="8" x2="12" y2="8"></line><line x1="3.95" y1="6.06" x2="8.54" y2="14"></line><line x1="10.88" y1="21.94" x2="15.46" y2="14"></line></svg>`,
  image: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="3" width="18" height="18" rx="2" ry="2"></rect><circle cx="8.5" cy="8.5" r="1.5"></circle><polyline points="21 15 16 10 5 21"></polyline></svg>`,
  music: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M9 18V5l12-2v13"></path><circle cx="6" cy="18" r="3"></circle><circle cx="18" cy="16" r="3"></circle></svg>`,
  tool: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M14.7 6.3a1 1 0 0 0 0 1.4l1.6 1.6a1 1 0 0 0 1.4 0l3.77-3.77a6 6 0 0 1-7.94 7.94l-6.91 6.91a2.12 2.12 0 0 1-3-3l6.91-6.91a6 6 0 0 1 7.94-7.94l-3.76 3.76z"></path></svg>`,
  folder: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"></path></svg>`,
  git: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="18" cy="18" r="3"></circle><circle cx="6" cy="6" r="3"></circle><path d="M18 15V9a9 9 0 0 0-9-9"></path><circle cx="6" cy="18" r="3"></circle><path d="M6 9v6"></path></svg>`,
  video: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="2" y="6" width="14" height="12" rx="2"></rect><polygon points="22 8 16 12 22 16 22 8"></polygon></svg>`,
  scissors: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="6" cy="6" r="3"></circle><circle cx="6" cy="18" r="3"></circle><line x1="20" y1="4" x2="8.12" y2="15.88"></line><line x1="14.47" y1="14.48" x2="20" y2="20"></line><line x1="8.12" y1="8.12" x2="12" y2="12"></line></svg>`,
  download: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"></path><polyline points="7 10 12 15 17 10"></polyline><line x1="12" y1="15" x2="12" y2="3"></line></svg>`,
  text: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="5" width="18" height="14" rx="2"></rect><line x1="7" y1="11" x2="13" y2="11"></line><line x1="7" y1="15" x2="17" y2="15"></line></svg>`,
  layers: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polygon points="12 2 2 7 12 12 22 7 12 2"></polygon><polyline points="2 17 12 22 22 17"></polyline><polyline points="2 12 12 17 22 12"></polyline></svg>`,
  sliders: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="4" y1="21" x2="4" y2="14"></line><line x1="4" y1="10" x2="4" y2="3"></line><line x1="12" y1="21" x2="12" y2="12"></line><line x1="12" y1="8" x2="12" y2="3"></line><line x1="20" y1="21" x2="20" y2="16"></line><line x1="20" y1="12" x2="20" y2="3"></line><line x1="1" y1="14" x2="7" y2="14"></line><line x1="9" y1="8" x2="15" y2="8"></line><line x1="17" y1="16" x2="23" y2="16"></line></svg>`
};

// Global App State
const state = {
  registry: null,
  tools: [],
  categories: [],
  activeCategory: 'all',
  searchQuery: '',
  platformFilter: 'all',
  sortBy: 'featured',
  detectedOS: 'windows',
  activeTool: null
};

// Detect User's Operating System
function detectOS() {
  const ua = window.navigator.userAgent.toLowerCase();
  const platform = window.navigator.platform ? window.navigator.platform.toLowerCase() : '';
  
  if (platform.includes('win') || ua.includes('windows')) {
    state.detectedOS = 'windows';
    return { name: 'Windows', id: 'windows-amd64' };
  } else if (platform.includes('mac') || ua.includes('macintosh')) {
    state.detectedOS = 'darwin';
    return { name: 'macOS', id: 'darwin-arm64' };
  } else if (platform.includes('linux') || ua.includes('linux')) {
    state.detectedOS = 'linux';
    return { name: 'Linux', id: 'linux-amd64' };
  }
  return { name: 'Cross-Platform', id: 'all' };
}

// Fetch GitHub Stars
async function fetchRepoStars() {
  try {
    const res = await fetch('https://api.github.com/repos/ranjanlive/ZTOOLS');
    if (res.ok) {
      const data = await res.json();
      const countEl = document.getElementById('starCount');
      if (countEl && data.stargazers_count !== undefined) {
        countEl.textContent = `★ ${data.stargazers_count}`;
      }
    }
  } catch (e) {
    console.warn('Could not fetch GitHub stars:', e);
  }
}

// Load Registry JSON with automatic fallbacks
async function loadRegistry() {
  const loadingEl = document.getElementById('loadingState');
  if (loadingEl) loadingEl.style.display = 'block';

  const urls = [
    './registry.json',
    '../registry.json',
    'https://raw.githubusercontent.com/ranjanlive/ZTOOLS/main/registry.json'
  ];

  let loaded = false;
  for (const url of urls) {
    try {
      const res = await fetch(url);
      if (res.ok) {
        state.registry = await res.json();
        loaded = true;
        break;
      }
    } catch (e) {
      console.warn(`Failed loading from ${url}, trying next...`);
    }
  }

  if (loadingEl) loadingEl.style.display = 'none';

  if (!loaded || !state.registry) {
    console.error('All registry fetch attempts failed.');
    document.getElementById('emptyState').style.display = 'block';
    return;
  }

  state.tools = state.registry.tools || [];
  state.categories = state.registry.categories || [];

  updateStats();
  renderCategories();
  renderTools();
}

// Update Top Counters
function updateStats() {
  const toolsCountEl = document.getElementById('totalToolsCount');
  const catCountEl = document.getElementById('categoriesCount');
  if (toolsCountEl) toolsCountEl.textContent = state.tools.length;
  if (catCountEl) catCountEl.textContent = state.categories.length;
}

// Render Category Filter Pills
function renderCategories() {
  const container = document.getElementById('categoryPills');
  if (!container) return;

  const html = [`<button class="pill-btn ${state.activeCategory === 'all' ? 'active' : ''}" data-category="all">All Tools (${state.tools.length})</button>`];

  state.categories.forEach(cat => {
    const count = state.tools.filter(t => t.category === cat.id).length;
    const isActive = state.activeCategory === cat.id ? 'active' : '';
    html.push(`<button class="pill-btn ${isActive}" data-category="${cat.id}">${cat.name} (${count})</button>`);
  });

  container.innerHTML = html.join('');

  container.querySelectorAll('.pill-btn').forEach(btn => {
    btn.addEventListener('click', () => {
      container.querySelectorAll('.pill-btn').forEach(b => b.classList.remove('active'));
      btn.classList.add('active');
      state.activeCategory = btn.getAttribute('data-category');
      renderTools();
    });
  });
}

// Filter and Sort Tools
function getFilteredTools() {
  let list = [...state.tools];

  // Category filter
  if (state.activeCategory !== 'all') {
    list = list.filter(t => t.category === state.activeCategory);
  }

  // OS Platform filter
  if (state.platformFilter !== 'all') {
    list = list.filter(t => {
      if (!t.platforms || t.platforms.length === 0) return true;
      return t.platforms.some(p => p.toLowerCase().includes(state.platformFilter));
    });
  }

  // Search Query filter
  if (state.searchQuery.trim() !== '') {
    const q = state.searchQuery.toLowerCase().trim();
    list = list.filter(t => {
      const matchName = t.name && t.name.toLowerCase().includes(q);
      const matchId = t.id && t.id.toLowerCase().includes(q);
      const matchDesc = t.description && t.description.toLowerCase().includes(q);
      const matchAuthor = t.author && t.author.name && t.author.name.toLowerCase().includes(q);
      const matchTags = t.tags && t.tags.some(tag => tag.toLowerCase().includes(q));
      return matchName || matchId || matchDesc || matchAuthor || matchTags;
    });
  }

  // Sorting
  if (state.sortBy === 'name') {
    list.sort((a, b) => a.name.localeCompare(b.name));
  } else if (state.sortBy === 'version') {
    list.sort((a, b) => b.version.localeCompare(a.version));
  }

  return list;
}

// Render Tools Grid
function renderTools() {
  const grid = document.getElementById('toolsGrid');
  const emptyState = document.getElementById('emptyState');
  if (!grid) return;

  const filtered = getFilteredTools();

  if (filtered.length === 0) {
    grid.innerHTML = '';
    if (emptyState) emptyState.style.display = 'block';
    return;
  }

  if (emptyState) emptyState.style.display = 'none';

  grid.innerHTML = filtered.map(tool => {
    const iconSvg = ICONS[tool.icon] || ICONS.tool;
    const authorName = tool.author ? tool.author.name : 'ZTools Community';
    const authorUrl = tool.author ? tool.author.url : '#';
    const tagsHtml = (tool.tags || []).slice(0, 4).map(t => `<span class="tag-pill">#${escapeHtml(t)}</span>`).join('');

    // Primary download url matching detected OS
    let downloadUrl = '#';
    if (tool.downloads) {
      if (state.detectedOS === 'windows' && tool.downloads['windows-amd64']) {
        downloadUrl = tool.downloads['windows-amd64'];
      } else if (state.detectedOS === 'darwin' && (tool.downloads['darwin-arm64'] || tool.downloads['darwin-amd64'])) {
        downloadUrl = tool.downloads['darwin-arm64'] || tool.downloads['darwin-amd64'];
      } else if (state.detectedOS === 'linux' && tool.downloads['linux-amd64']) {
        downloadUrl = tool.downloads['linux-amd64'];
      } else {
        downloadUrl = Object.values(tool.downloads)[0] || '#';
      }
    }

    return `
      <div class="tool-card" data-tool-id="${escapeHtml(tool.id)}">
        <div class="tool-header">
          <div class="tool-title-wrapper">
            <div class="tool-avatar">${iconSvg}</div>
            <div class="tool-name-group">
              <h3>${escapeHtml(tool.name)}</h3>
              <span class="tool-id">${escapeHtml(tool.id)}</span>
            </div>
          </div>
          <span class="tool-version-badge">v${escapeHtml(tool.version)}</span>
        </div>

        <p class="tool-description">${escapeHtml(tool.description)}</p>

        <div class="tool-tags">${tagsHtml}</div>

        <div class="tool-footer">
          <div class="tool-meta">
            <span>By <a href="${escapeHtml(authorUrl)}" target="_blank" class="author-link">${escapeHtml(authorName)}</a></span>
          </div>
          <div class="tool-actions">
            <button class="btn-card-action" onclick="openToolModal('${escapeHtml(tool.id)}')">
              Inspect
            </button>
            <a href="${escapeHtml(downloadUrl)}" class="btn-card-action btn-card-primary" title="Download for ${state.detectedOS}">
              Download
            </a>
          </div>
        </div>
      </div>
    `;
  }).join('');
}

// Open Tool Details Modal
async function openToolModal(toolId) {
  const tool = state.tools.find(t => t.id === toolId);
  if (!tool) return;
  state.activeTool = tool;

  const modal = document.getElementById('modalOverlay');
  const titleEl = document.getElementById('modalTitle');
  const iconEl = document.getElementById('modalIcon');
  const metaEl = document.getElementById('modalMeta');
  const descEl = document.getElementById('modalDescription');
  const tableBody = document.getElementById('schemaTableBody');
  const sampleCode = document.getElementById('sampleCommandCode');
  const manifestCode = document.getElementById('rawManifestCode');
  const manifestFileName = document.getElementById('manifestFileName');
  const downloadsList = document.getElementById('downloadsList');
  const sourceBtn = document.getElementById('modalSourceBtn');

  titleEl.textContent = tool.name;
  iconEl.innerHTML = ICONS[tool.icon] || ICONS.tool;
  metaEl.textContent = `${tool.id} • v${tool.version} • ${tool.license || 'Apache-2.0'}`;
  descEl.textContent = tool.description;
  sourceBtn.href = tool.source_url || 'https://github.com/ranjanlive/ZTOOLS';
  manifestFileName.textContent = `${tool.id}.json`;

  // Sample Execution
  const sampleJson = JSON.stringify(tool.sample_call || {}, null, 2);
  sampleCode.textContent = `zagent tools run ${tool.id} --args '${sampleJson}'`;

  // Downloads List
  if (tool.downloads && Object.keys(tool.downloads).length > 0) {
    downloadsList.innerHTML = Object.entries(tool.downloads).map(([platform, url]) => `
      <div class="download-row">
        <div class="download-info">
          <span class="download-platform">${escapeHtml(platform)}</span>
        </div>
        <a href="${escapeHtml(url)}" target="_blank" class="btn btn-secondary" style="font-size: 0.8rem; padding: 0.35rem 0.75rem;">
          Download Archive
        </a>
      </div>
    `).join('');
  } else {
    downloadsList.innerHTML = `<p style="color: var(--text-muted);">Install via ZAgent: <code>${escapeHtml(tool.install_command || 'zagent tools install ' + tool.id)}</code></p>`;
  }

  // Fetch Manifest to populate Schema & Raw Manifest
  tableBody.innerHTML = `<tr><td colspan="4" style="text-align: center; color: var(--text-muted); padding: 1.5rem;">Loading schema from ${escapeHtml(tool.manifest_path)}...</td></tr>`;
  manifestCode.textContent = 'Loading manifest...';

  modal.classList.add('open');

  try {
    let manifestData = null;
    const pathsToTry = [
      tool.manifest_path,
      `../${tool.manifest_path}`,
      `https://raw.githubusercontent.com/ranjanlive/ZTOOLS/main/${tool.manifest_path}`
    ];

    for (const p of pathsToTry) {
      try {
        const res = await fetch(p);
        if (res.ok) {
          manifestData = await res.json();
          break;
        }
      } catch (err) {}
    }

    if (manifestData) {
      manifestCode.textContent = JSON.stringify(manifestData, null, 2);

      // Render Parameters Table
      const params = manifestData.parameters?.properties || {};
      const requiredList = manifestData.parameters?.required || [];
      const paramKeys = Object.keys(params);

      if (paramKeys.length === 0) {
        tableBody.innerHTML = `<tr><td colspan="4" style="text-align: center; color: var(--text-muted);">No input parameters required for this tool.</td></tr>`;
      } else {
        tableBody.innerHTML = paramKeys.map(key => {
          const p = params[key];
          const isReq = requiredList.includes(key);
          const pType = p.type || (p.enum ? 'enum' : 'any');
          const pDesc = p.description || 'No description provided';
          return `
            <tr>
              <td><span class="param-name">${escapeHtml(key)}</span></td>
              <td><span class="param-type">${escapeHtml(pType)}</span></td>
              <td>${isReq ? '<span class="req-badge">Required</span>' : '<span class="opt-badge">Optional</span>'}</td>
              <td>${escapeHtml(pDesc)}</td>
            </tr>
          `;
        }).join('');
      }
    } else {
      manifestCode.textContent = 'Could not load manifest file. Check repository path.';
      tableBody.innerHTML = `<tr><td colspan="4" style="text-align: center; color: var(--text-muted);">Manifest could not be retrieved.</td></tr>`;
    }
  } catch (err) {
    manifestCode.textContent = 'Error fetching manifest: ' + err.message;
  }
}

// Close Modal
function closeModal() {
  const modal = document.getElementById('modalOverlay');
  if (modal) modal.classList.remove('open');
}

// Show Toast
function showToast(message = 'Copied to clipboard!') {
  const toast = document.getElementById('toast');
  if (!toast) return;
  toast.textContent = message;
  toast.classList.add('show');
  setTimeout(() => toast.classList.remove('show'), 2200);
}

// Copy to Clipboard Helper
function copyText(text, successMsg = 'Copied to clipboard!') {
  if (navigator.clipboard) {
    navigator.clipboard.writeText(text).then(() => showToast(successMsg));
  } else {
    const el = document.createElement('textarea');
    el.value = text;
    document.body.appendChild(el);
    el.select();
    document.execCommand('copy');
    document.body.removeChild(el);
    showToast(successMsg);
  }
}

// HTML Escaper
function escapeHtml(str) {
  if (!str) return '';
  return String(str)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

// Initialization and Event Listeners
document.addEventListener('DOMContentLoaded', () => {
  const detected = detectOS();
  const osBadge = document.getElementById('detectedOsText');
  if (osBadge) osBadge.textContent = detected.name;

  fetchRepoStars();
  loadRegistry();

  // Search input listeners
  const searchInput = document.getElementById('searchInput');
  const searchClear = document.getElementById('searchClear');

  if (searchInput) {
    searchInput.addEventListener('input', (e) => {
      state.searchQuery = e.target.value;
      if (searchClear) searchClear.style.display = state.searchQuery ? 'block' : 'none';
      renderTools();
    });
  }

  if (searchClear) {
    searchClear.addEventListener('click', () => {
      if (searchInput) searchInput.value = '';
      state.searchQuery = '';
      searchClear.style.display = 'none';
      renderTools();
    });
  }

  // Filter dropdown listeners
  const platformFilter = document.getElementById('platformFilter');
  if (platformFilter) {
    platformFilter.addEventListener('change', (e) => {
      state.platformFilter = e.target.value;
      renderTools();
    });
  }

  const sortBy = document.getElementById('sortBy');
  if (sortBy) {
    sortBy.addEventListener('change', (e) => {
      state.sortBy = e.target.value;
      renderTools();
    });
  }

  // Reset Filters button
  const resetBtn = document.getElementById('resetSearchBtn');
  if (resetBtn) {
    resetBtn.addEventListener('click', () => {
      state.searchQuery = '';
      state.activeCategory = 'all';
      state.platformFilter = 'all';
      if (searchInput) searchInput.value = '';
      if (platformFilter) platformFilter.value = 'all';
      if (searchClear) searchClear.style.display = 'none';
      renderCategories();
      renderTools();
    });
  }

  // Hero Copy Button
  const heroCopyBtn = document.getElementById('heroCopyBtn');
  const heroCode = document.getElementById('heroCode');
  if (heroCopyBtn && heroCode) {
    heroCopyBtn.addEventListener('click', () => {
      copyText(heroCode.textContent);
    });
  }

  // Modal Close Listeners
  const modalCloseBtn = document.getElementById('modalCloseBtn');
  const modalOverlay = document.getElementById('modalOverlay');

  if (modalCloseBtn) modalCloseBtn.addEventListener('click', closeModal);
  if (modalOverlay) {
    modalOverlay.addEventListener('click', (e) => {
      if (e.target === modalOverlay) closeModal();
    });
  }

  document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape') closeModal();
  });

  // Modal Tab Switching
  const modalTabs = document.querySelectorAll('.modal-tab');
  modalTabs.forEach(tab => {
    tab.addEventListener('click', () => {
      modalTabs.forEach(t => t.classList.remove('active'));
      tab.classList.add('active');

      const targetTab = tab.getAttribute('data-tab');
      document.querySelectorAll('.tab-pane').forEach(pane => pane.classList.remove('active'));

      if (targetTab === 'schema') document.getElementById('tabSchema')?.classList.add('active');
      if (targetTab === 'sample') document.getElementById('tabSample')?.classList.add('active');
      if (targetTab === 'manifest') document.getElementById('tabManifest')?.classList.add('active');
      if (targetTab === 'downloads') document.getElementById('tabDownloads')?.classList.add('active');
    });
  });

  // Modal Copy Buttons
  const copySampleBtn = document.getElementById('copySampleBtn');
  if (copySampleBtn) {
    copySampleBtn.addEventListener('click', () => {
      const code = document.getElementById('sampleCommandCode')?.textContent;
      if (code) copyText(code);
    });
  }

  const copyManifestBtn = document.getElementById('copyManifestBtn');
  if (copyManifestBtn) {
    copyManifestBtn.addEventListener('click', () => {
      const code = document.getElementById('rawManifestCode')?.textContent;
      if (code) copyText(code, 'Manifest JSON copied!');
    });
  }

  const modalInstallCopyBtn = document.getElementById('modalInstallCopyBtn');
  if (modalInstallCopyBtn) {
    modalInstallCopyBtn.addEventListener('click', () => {
      if (state.activeTool) {
        const cmd = state.activeTool.install_command || `zagent tools install ${state.activeTool.id}`;
        copyText(cmd, 'Install command copied!');
      }
    });
  }
});
