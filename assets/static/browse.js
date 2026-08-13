// 读取页逻辑：选目录 → 列文件 → 下载 / 预览
const $ = id => document.getElementById(id);
const IMG_EXTS = ['png','jpg','jpeg','gif','webp','bmp','heic'];
const VID_EXTS = ['mp4','mov','m4v','webm'];

// 图标 SVG（跨平台渲染一致）
const SVG_FOLDER  = '<svg class="icon" viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"/></svg>';
const SVG_FILE    = '<svg class="icon" viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/></svg>';
const SVG_DOWNLOAD = '<svg class="icon" viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>';

let currentDir = '';
let previewFiles = []; // [{name, previewUrl, ext}, ...] 当前目录可预览文件
let lbIndex = 0;       // 灯箱当前索引

function show(view) {
  $('dirsView').classList.toggle('active', view === 'dirs');
  $('filesView').classList.toggle('active', view === 'files');
}

// ---- 加载目录列表 ----
async function loadDirs() {
  $('dirsLoading').style.display = 'block';
  $('dirsList').innerHTML = '';
  $('dirsEmpty').style.display = 'none';
  try {
    const res = await fetch('/api/dirs');
    const data = await res.json();
    $('dirsLoading').style.display = 'none';
    if (!data.dirs.length) {
      $('dirsEmpty').style.display = 'block';
      return;
    }
    $('dirsList').innerHTML = data.dirs.map(d => `
      <div class="dir-card" onclick="selectDir('${escapeHtml(d.name)}')">
        <div class="dicon">${SVG_FOLDER}</div>
        <div>
          <div class="dname">${escapeHtml(d.name)}</div>
          <div class="dcount">${d.count} 个文件</div>
        </div>
        <div class="arrow">›</div>
      </div>
    `).join('');
  } catch (e) {
    $('dirsLoading').textContent = '加载失败: ' + e.message;
  }
}

// ---- 选择目录，加载文件 ----
async function selectDir(name) {
  currentDir = name;
  $('currentDir').textContent = name;
  show('files');
  $('filesLoading').style.display = 'block';
  $('filesList').innerHTML = '';
  $('filesEmpty').style.display = 'none';
  try {
    const res = await fetch('/api/files?dir=' + encodeURIComponent(name));
    const data = await res.json();
    $('filesLoading').style.display = 'none';
    if (data.error) { $('filesList').innerHTML = '<div class="empty">' + data.error + '</div>'; return; }
    if (!data.files.length) { $('filesEmpty').style.display = 'block'; return; }
    // 构建可预览文件列表（图片 + 视频），供灯箱左右切换
    previewFiles = data.files.filter(f => {
      const ext = f.name.split('.').pop().toLowerCase();
      return IMG_EXTS.includes(ext) || VID_EXTS.includes(ext);
    }).map(f => {
      const u = '/api/download?dir=' + encodeURIComponent(name) + '&file=' + encodeURIComponent(f.name);
      return { name: f.name, previewUrl: u + '&inline=1', ext: f.name.split('.').pop().toLowerCase() };
    });
    $('filesList').innerHTML = data.files.map(renderFile).join('');
    // 显示批量操作栏，重置选中状态
    $('batchBar').style.display = 'flex';
    $('selectAll').checked = false;
    updateSelectedCount();
  } catch (e) {
    $('filesLoading').textContent = '加载失败: ' + e.message;
  }
}

function renderFile(f) {
  const url = '/api/download?dir=' + encodeURIComponent(currentDir) + '&file=' + encodeURIComponent(f.name);
  const previewUrl = url + '&inline=1';
  const meta = formatSize(f.size) + ' · ' + formatTime(f.mtime);
  const ext = f.name.split('.').pop().toLowerCase();
  const isImg = IMG_EXTS.includes(ext);
  const isVid = VID_EXTS.includes(ext);
  let thumb, preview = '';
  if (isImg || isVid) {
    const idx = previewFiles.findIndex(p => p.name === f.name);
    if (isImg) {
      thumb = `<img class="thumb" src="${url}" alt="" loading="lazy" style="cursor:pointer" onclick="openLightbox(${idx})">`;
    } else {
      thumb = `<video class="thumb-video" src="${url}" preload="metadata" style="cursor:pointer" onclick="openLightbox(${idx})"></video>`;
    }
    preview = `<a class="preview-link" onclick="openLightbox(${idx})">${isImg ? '预览' : '播放'}</a>`;
  } else {
    thumb = `<div class="thumb-other">${SVG_FILE}</div>`;
  }
  return `
    <div class="file-item">
      <input type="checkbox" class="file-check" data-name="${escapeAttr(f.name)}" onchange="updateSelectedCount()">
      ${thumb}
      <div class="info">
        <div class="fname">${escapeHtml(f.name)}</div>
        <div class="fmeta">${meta}</div>
        <a class="dl-btn" data-url="${url}" data-name="${escapeAttr(f.name)}" onclick="onSingleDownload(this)">下载</a>
        ${preview}
      </div>
    </div>
  `;
}

// ---- 批量下载 ----
function toggleSelectAll(checked) {
  document.querySelectorAll('.file-check').forEach(c => { c.checked = checked; });
  updateSelectedCount();
}

function updateSelectedCount() {
  const n = document.querySelectorAll('.file-check:checked').length;
  $('selectedCount').textContent = n;
  $('btnSelected').disabled = n === 0;
}

async function downloadAll(btn) { await downloadBatch([], btn); }

async function downloadSelected(btn) {
  const files = Array.from(document.querySelectorAll('.file-check:checked')).map(c => c.dataset.name);
  if (!files.length) return;
  await downloadBatch(files, btn);
}

// 单文件下载：fetch + blob，下载完成在 finally 立即恢复按钮，精确感知不靠定时器猜
async function onSingleDownload(btn) {
  if (btn.classList.contains('loading')) return;
  btn.classList.add('loading');
  btn.innerHTML = '<span class="spinner"></span>下载中';
  try {
    const res = await fetch(btn.dataset.url);
    if (!res.ok) { alert('下载失败: ' + res.status); return; }
    const blob = await res.blob();
    const a = document.createElement('a');
    a.href = URL.createObjectURL(blob);
    a.download = btn.dataset.name;
    a.click();
    URL.revokeObjectURL(a.href);
  } catch (e) {
    alert('下载失败: ' + e.message);
  } finally {
    btn.classList.remove('loading');
    btn.textContent = '下载';
  }
}

async function downloadBatch(files, triggerBtn) {
  const btns = document.querySelectorAll('.btn-batch');
  const states = Array.from(btns).map(b => ({ el: b, html: b.innerHTML }));
  btns.forEach(b => { b.disabled = true; });
  if (triggerBtn) {
    triggerBtn.classList.add('loading');
    triggerBtn.innerHTML = '<span class="spinner"></span>下载中...';
  }
  try {
    const res = await fetch('/api/download-batch', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ dir: currentDir, files: files }),
    });
    if (!res.ok) { alert('下载失败: ' + res.status); return; }
    const blob = await res.blob();
    const a = document.createElement('a');
    a.href = URL.createObjectURL(blob);
    a.download = currentDir + '.zip';
    a.click();
    URL.revokeObjectURL(a.href);
  } catch (e) {
    alert('下载失败: ' + e.message);
  } finally {
    btns.forEach(b => { b.disabled = false; b.classList.remove('loading'); b.innerHTML = states.find(s => s.el === b).html; });
    updateSelectedCount();
  }
}

// ---- 灯箱预览：左右切换 + 键盘导航 ----
function openLightbox(idx) {
  if (idx < 0 || idx >= previewFiles.length) return;
  lbIndex = idx;
  showLbImage();
  $('lightbox').style.display = 'flex';
  document.body.style.overflow = 'hidden';
  document.addEventListener('keydown', lbKeyHandler);
}

function closeLightbox() {
  $('lightbox').style.display = 'none';
  $('lbVideo').pause();
  document.body.style.overflow = '';
  document.removeEventListener('keydown', lbKeyHandler);
}

function lbNav(delta) {
  lbIndex = (lbIndex + delta + previewFiles.length) % previewFiles.length;
  showLbImage();
}

function showLbImage() {
  const f = previewFiles[lbIndex];
  const isVid = VID_EXTS.includes(f.ext);
  const lbImg = $('lbImg');
  const lbVideo = $('lbVideo');
  lbImg.style.display = isVid ? 'none' : 'block';
  lbVideo.style.display = isVid ? 'block' : 'none';
  if (isVid) { lbVideo.src = f.previewUrl; }
  else { lbImg.src = f.previewUrl; }
  $('lbCounter').textContent = `${lbIndex + 1} / ${previewFiles.length}`;
  $('lbName').textContent = f.name;
  // 仅 1 个文件时隐藏左右箭头
  const showNav = previewFiles.length > 1;
  $('lbPrev').style.display = showNav ? 'flex' : 'none';
  $('lbNext').style.display = showNav ? 'flex' : 'none';
  // 重置下载按钮状态
  const dlBtn = $('lbDownload');
  dlBtn.classList.remove('loading');
  dlBtn.disabled = false;
  dlBtn.innerHTML = SVG_DOWNLOAD + ' 下载';
}

// 灯箱内下载当前文件：fetch + blob，下载中禁用按钮显示 loading
async function lbDownload() {
  const btn = $('lbDownload');
  if (btn.classList.contains('loading')) return;
  const f = previewFiles[lbIndex];
  const url = f.previewUrl.replace('&inline=1', '');
  btn.classList.add('loading');
  btn.disabled = true;
  btn.innerHTML = '<span class="spinner"></span>下载中';
  try {
    const res = await fetch(url);
    if (!res.ok) { alert('下载失败: ' + res.status); return; }
    const blob = await res.blob();
    const a = document.createElement('a');
    a.href = URL.createObjectURL(blob);
    a.download = f.name;
    a.click();
    URL.revokeObjectURL(a.href);
  } catch (e) {
    alert('下载失败: ' + e.message);
  } finally {
    btn.classList.remove('loading');
    btn.disabled = false;
    btn.innerHTML = SVG_DOWNLOAD + ' 下载';
  }
}

function lbKeyHandler(e) {
  if (e.key === 'ArrowLeft') lbNav(-1);
  else if (e.key === 'ArrowRight') lbNav(1);
  else if (e.key === 'Escape') closeLightbox();
}

function backToDirs() { show('dirs'); }

function formatSize(b) {
  if (b < 1024) return b + ' B';
  if (b < 1048576) return (b/1024).toFixed(1) + ' KB';
  if (b < 1073741824) return (b/1048576).toFixed(2) + ' MB';
  return (b/1073741824).toFixed(2) + ' GB';
}
function formatTime(ts) {
  const d = new Date(ts * 1000);
  const p = n => String(n).padStart(2,'0');
  return `${d.getFullYear()}-${p(d.getMonth()+1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`;
}
function escapeHtml(s) { return s.replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c])); }
function escapeAttr(s) { return s.replace(/"/g, '&quot;'); }

loadDirs();
