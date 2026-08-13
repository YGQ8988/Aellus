// 读取页逻辑：选目录 → 列文件 → 下载 / 预览
const $ = id => document.getElementById(id);
const IMG_EXTS = ['png','jpg','jpeg','gif','webp','bmp','heic'];
const VID_EXTS = ['mp4','mov','m4v','webm'];

// 图标 SVG（跨平台渲染一致）
const SVG_FOLDER  = '<svg class="icon" viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"/></svg>';
const SVG_FILE    = '<svg class="icon" viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/></svg>';
const SVG_DOWNLOAD = '<svg class="icon" viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>';
const SVG_TRASH    = '<svg class="icon" viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>';

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
        <a class="dl-btn" href="${url}" download="${escapeAttr(f.name)}">下载</a>
        ${preview}
        <a class="del-link" data-name="${escapeAttr(f.name)}" onclick="deleteFile(this.dataset.name, this)">删除</a>
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
  $('btnDeleteSelected').disabled = n === 0;
}

async function downloadAll(btn) { await downloadBatch([], btn); }

async function downloadSelected(btn) {
  const files = Array.from(document.querySelectorAll('.file-check:checked')).map(c => c.dataset.name);
  if (!files.length) return;
  await downloadBatch(files, btn);
}

// 单文件下载：直接导航到下载 URL，后端 Content-Disposition: attachment 强制下载。
// 同步触发保留用户手势上下文，兼容安卓 WebView；不 fetch blob 避免大文件 OOM。
function onSingleDownload(btn) {
  if (btn.classList.contains('loading')) return;
  btn.classList.add('loading');
  btn.innerHTML = '<span class="spinner"></span>下载中';
  const a = document.createElement('a');
  a.href = btn.dataset.url;
  a.download = btn.dataset.name;
  document.body.appendChild(a);
  a.click();
  a.remove();
  // 浏览器原生下载无法精确感知完成，延时恢复按钮
  setTimeout(() => {
    btn.classList.remove('loading');
    btn.textContent = '下载';
  }, 1500);
}

async function downloadBatch(files, triggerBtn) {
  const btns = document.querySelectorAll('.btn-batch');
  const states = Array.from(btns).map(b => ({ el: b, html: b.innerHTML }));
  btns.forEach(b => { b.disabled = true; });
  if (triggerBtn) {
    triggerBtn.classList.add('loading');
    triggerBtn.innerHTML = '<span class="spinner"></span>下载中...';
  }
  // 用隐藏 form POST 触发浏览器原生下载（非 blob），兼容 Alook 等安卓浏览器
  const form = document.createElement('form');
  form.method = 'POST';
  form.action = '/api/download-batch';
  const addField = (name, value) => {
    const input = document.createElement('input');
    input.type = 'hidden'; input.name = name; input.value = value;
    form.appendChild(input);
  };
  addField('dir', currentDir);
  files.forEach(f => addField('files', f));
  document.body.appendChild(form);
  form.submit();
  form.remove();
  // 原生下载无法精确感知完成，短暂 loading 后恢复
  setTimeout(() => {
    btns.forEach(b => { b.disabled = false; b.classList.remove('loading'); b.innerHTML = states.find(s => s.el === b).html; });
    updateSelectedCount();
  }, 2000);
}

// ---- 删除 ----
// 调用后端 /api/delete，成功后重新加载当前目录文件列表。
async function deleteFiles(names) {
  const res = await fetch('/api/delete', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ dir: currentDir, files: names })
  });
  const data = await res.json();
  return data;
}

// 删除单个文件（文件列表页）
async function deleteFile(name, btn) {
  if (!confirm(`确认删除文件 “${name}” 吗？删除后不可恢复。`)) return;
  if (btn) { btn.classList.add('loading'); btn.textContent = '删除中'; }
  try {
    const data = await deleteFiles([name]);
    if (data.ok && data.deleted.includes(name)) {
      selectDir(currentDir);  // 重新加载列表
    } else {
      alert('删除失败: ' + (data.failed && data.failed[0] ? data.failed[0].error : '未知错误'));
      if (btn) { btn.classList.remove('loading'); btn.textContent = '删除'; }
    }
  } catch (e) {
    alert('删除失败: ' + e.message);
    if (btn) { btn.classList.remove('loading'); btn.textContent = '删除'; }
  }
}

// 删除选中的文件（批量操作栏）
async function deleteSelected(btn) {
  const files = Array.from(document.querySelectorAll('.file-check:checked')).map(c => c.dataset.name);
  if (!files.length) return;
  if (!confirm(`确认删除选中的 ${files.length} 个文件吗？删除后不可恢复。`)) return;
  const origHtml = btn.innerHTML;
  btn.classList.add('loading');
  btn.disabled = true;
  btn.innerHTML = '<span class="spinner"></span>删除中...';
  try {
    const data = await deleteFiles(files);
    const ok = data.deleted.length;
    const fail = data.failed.length;
    if (fail > 0) {
      alert(`成功删除 ${ok} 个，失败 ${fail} 个: ` + data.failed.map(f => f.name + '(' + f.error + ')').join(', '));
    }
    await selectDir(currentDir);  // 重新加载列表（内部会 updateSelectedCount 重置按钮 disabled）
  } catch (e) {
    alert('删除失败: ' + e.message);
  } finally {
    // 无论成功失败都恢复按钮视觉状态；loading class 含 pointer-events:none 会锁死按钮，必须清除。
    // disabled 交由 updateSelectedCount 根据当前选中数决定。
    btn.classList.remove('loading');
    btn.innerHTML = origHtml;
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
  // 重置删除按钮状态
  const delBtn = $('lbDelete');
  delBtn.classList.remove('loading');
  delBtn.disabled = false;
  delBtn.innerHTML = SVG_TRASH + ' 删除';
}

// 灯箱内下载当前文件：直接导航到下载 URL，浏览器原生下载，兼容 Alook 等安卓浏览器
function lbDownload() {
  const btn = $('lbDownload');
  if (btn.classList.contains('loading')) return;
  const f = previewFiles[lbIndex];
  const url = f.previewUrl.replace('&inline=1', '');
  btn.classList.add('loading');
  btn.disabled = true;
  btn.innerHTML = '<span class="spinner"></span>下载中';
  // 直接导航触发浏览器原生下载（不依赖 a.click()，兼容安卓浏览器）
  window.location.href = url;
  setTimeout(() => {
    btn.classList.remove('loading');
    btn.disabled = false;
    btn.innerHTML = SVG_DOWNLOAD + ' 下载';
  }, 1500);
}

// 灯箱内删除当前文件：删除后从 previewFiles 移除，跳到下一张；若已无文件则关闭灯箱。
async function lbDelete() {
  const btn = $('lbDelete');
  if (btn.classList.contains('loading')) return;
  const f = previewFiles[lbIndex];
  if (!confirm(`确认删除文件 “${f.name}” 吗？删除后不可恢复。`)) return;
  btn.classList.add('loading');
  btn.disabled = true;
  btn.innerHTML = '<span class="spinner"></span>删除中';
  try {
    const data = await deleteFiles([f.name]);
    if (!(data.ok && data.deleted.includes(f.name))) {
      alert('删除失败: ' + (data.failed && data.failed[0] ? data.failed[0].error : '未知错误'));
      btn.classList.remove('loading');
      btn.disabled = false;
      btn.innerHTML = SVG_TRASH + ' 删除';
      return;
    }
    // 从预览列表移除已删文件
    previewFiles.splice(lbIndex, 1);
    if (previewFiles.length === 0) {
      closeLightbox();
    } else {
      if (lbIndex >= previewFiles.length) lbIndex = previewFiles.length - 1;
      showLbImage();
    }
    // 后台刷新文件列表
    selectDir(currentDir);
  } catch (e) {
    alert('删除失败: ' + e.message);
    btn.classList.remove('loading');
    btn.disabled = false;
    btn.innerHTML = SVG_TRASH + ' 删除';
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
