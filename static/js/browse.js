// 读取页逻辑：选目录 → 列文件 → 下载 / 预览
const $ = id => document.getElementById(id);
const IMG_EXTS = ['png','jpg','jpeg','gif','webp','bmp','heic'];
const VID_EXTS = ['mp4','mov','m4v','webm'];

// 图标 SVG（跨平台渲染一致）
const SVG_FOLDER  = '<svg class="icon" viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"/></svg>';
const SVG_FILE    = '<svg class="icon" viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/></svg>';
const SVG_DOWNLOAD = '<svg class="icon" viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>';

let currentDir = '';
let allDirs = []; // 最近一次 /api/dirs 返回的目录列表，用于决定面包屑是否显示「目录」层级
let previewFiles = []; // [{name, previewUrl, ext, deletable}, ...] 当前目录可预览文件
let lbIndex = 0;       // 灯箱当前索引
// 某项是否可删：服务端已按「上传来源 IP/UA 签名」算好并返回 deletable 字段，前端直接使用。
const canDel = d => !!(d && d.deletable);

function show(view) {
  $('dirsView').classList.toggle('active', view === 'dirs');
  $('filesView').classList.toggle('active', view === 'files');
  // 切换视图时清空另一视图的勾选，避免跨视图污染批量操作
  clearChecks(view === 'dirs' ? 'filesList' : 'dirsList');
  updateSelectedCount();
  if (typeof syncTopBarBlur === 'function') syncTopBarBlur();
}

// ---- 加载目录列表 ----
async function loadDirs(autoEnter = true) {
  $('dirsLoading').style.display = 'block';
  $('dirsList').innerHTML = '';
  $('dirsEmpty').style.display = 'none';
  try {
    const res = await fetch('/api/dirs');
    const data = await res.json();
    allDirs = data.dirs;
    $('dirsLoading').style.display = 'none';
    if (!data.dirs.length) {
      $('dirsEmpty').style.display = 'block';
      return;
    }
    $('dirsList').innerHTML = data.dirs.map(d => {
      const delable = canDel(d);
      return `
      <div class="file-item dir-card">
        <input type="checkbox" class="file-check checkbox" data-name="${escapeAttr(d.name)}" data-del="${delable}" onchange="updateSelectedCount()" onclick="event.stopPropagation()">
        <div class="file-right">
          <div class="file-main" onclick="selectDir('${escapeAttr(d.name)}')" style="cursor:pointer">
            <div class="thumb-folder">${SVG_FOLDER}</div>
            <div class="file-meta-col">
              <div class="fname fname-row"><span class="ftext">${escapeHtml(d.name === '' ? '未命名设备' : d.name)}</span><span class="badge badge-outline">文件夹</span></div>
              <div class="fmeta">${formatSize(d.size)} · ${formatCount(d.count)} 个文件 · ${formatDay(d.mtime)}</div>
            </div>
          </div>
          ${delable ? '<div class="file-actions"><a class="del-btn" data-name="' + escapeAttr(d.name) + '" onclick="event.stopPropagation(); onDeleteDir(this)">删除</a></div>' : ''}
        </div>
      </div>`;
    }).join('');
    // 显示目录页批量操作栏，重置全选
    $('dirsBatchBar').style.display = 'flex';
    $('selectAllDirs').checked = false;
    updateSelectedCount();
    // 刷新/重入时：若 sessionStorage 里保存了当前目录路径（且等于某个根目录或它的子路径），
    // 直接恢复到该目录，避免多目录场景刷新后回到目录选择页。
    // 仅在「没有任何记忆」时才按原规则：单目录自动进入，多目录展示目录选择页
    //（满足「返回首页后再读取文件应进目录页」的预期，因为返回首页会 clearAellusDir）。
    const saved = sessionStorage.getItem('aellus_currentDir');
    let target = null;
    if (autoEnter) {
      if (saved !== null && saved !== '') {
        // 仅当记忆的目录在当前（可能已切换）保存目录下真实存在时才恢复；
        // 否则视为失效（例如刚改过保存路径），交由下方兜底逻辑重新判定。
        const match = data.dirs.find(d => saved === d.name || saved.startsWith(d.name + '/'));
        if (match) target = saved;
      }
      // 兜底：无记忆、或记忆已失效时，若当前保存目录下恰好只有一个目录，则直接进入它。
      if (target === null && data.dirs.length === 1) {
        target = data.dirs[0].name;
      }
    }
    if (target !== null) selectDir(target);
  } catch (e) {
    $('dirsLoading').textContent = '加载失败: ' + e.message;
  }
}

// ---- 打开某个目录（支持子目录路径，如 "设备名/子目录"）----
async function openDir(path) {
  currentDir = path;
  try { sessionStorage.setItem('aellus_currentDir', path); } catch (e) {}
  show('files');
  // 首次（列表为空）才显示完整 loading 并清空；切换目录时保留旧列表、
  // 用半透明提示正在切换，避免整片清空出现的白屏“加载感”
  const firstLoad = $('filesList').children.length === 0;
  if (firstLoad) {
    $('filesLoading').style.display = 'block';
    $('filesList').innerHTML = '';
  } else {
    $('filesList').classList.add('swapping');
  }
  $('filesEmpty').style.display = 'none';
  try {
    const res = await fetch('/api/files?dir=' + encodeURIComponent(path));
    const data = await res.json();
    $('filesLoading').style.display = 'none';
    $('filesList').classList.remove('swapping');
    if (data.error) { $('filesList').innerHTML = '<div class="empty">' + data.error + '</div>'; buildBreadcrumb(); return; }
    if (!data.files.length) { $('filesList').innerHTML = ''; $('filesEmpty').style.display = 'block'; buildBreadcrumb(); return; }
    // 构建可预览文件列表（图片 + 视频），供灯箱左右切换（文件夹不进预览）
    previewFiles = data.files.filter(f => {
      if (f.isDir) return false;
      const ext = f.name.split('.').pop().toLowerCase();
      return IMG_EXTS.includes(ext) || VID_EXTS.includes(ext);
    }).map(f => {
      const u = '/api/download?dir=' + encodeURIComponent(path) + '&file=' + encodeURIComponent(f.name);
      return { name: f.name, previewUrl: u + '&inline=1', ext: f.name.split('.').pop().toLowerCase(), deletable: !!f.deletable };
    });
    $('filesList').innerHTML = data.files.map(renderFile).join('');
    // 显示批量操作栏，重置选中状态
    $('batchBar').style.display = 'flex';
    $('selectAll').checked = false;
    updateSelectedCount();
    buildBreadcrumb();
  } catch (e) {
    $('filesLoading').style.display = 'none';
    $('filesList').classList.remove('swapping');
    $('filesList').innerHTML = '<div class="empty">加载失败: ' + escapeHtml(e.message) + '</div>';
    buildBreadcrumb();
  }
}

// 进入某个目录（设备根目录或子目录路径）
function selectDir(name) { openDir(name); }
// 在当前目录下进入子文件夹
function enterFolder(name) { openDir(currentDir ? currentDir + '/' + name : name); }
// 返回首页时清掉“上次目录”记忆，使下次从首页进读取文件时走目录选择页
function clearAellusDir() { try { sessionStorage.removeItem('aellus_currentDir'); } catch (e) {} }

// 根据 currentDir（可能是多层路径）动态生成面包屑
function buildBreadcrumb() {
  const parts = ['<a class="bc-link bc-home" href="/" onclick="clearAellusDir()">← 返回首页</a>'];
  if (allDirs.length > 1) {
    parts.push('<span class="bc-sep">/</span>');
    parts.push('<a class="bc-link bc-dirs" href="javascript:backToDirs()">目录</a>');
  }
  const segs = (currentDir || '').split('/').filter(s => s !== '');
  if (segs.length === 0) {
    parts.push('<span class="bc-sep">/</span>');
    parts.push('<span class="bc-cur">' + (currentDir === '' ? '未命名设备' : escapeHtml(currentDir)) + '</span>');
  } else {
    let acc = '';
    segs.forEach((s, i) => {
      acc = acc ? acc + '/' + s : s;
      parts.push('<span class="bc-sep">/</span>');
      if (i === segs.length - 1) {
        parts.push('<span class="bc-cur">' + escapeHtml(s) + '</span>');
      } else {
        // 用 data-nav 存编码后的路径，onclick 再解码，避免 JSON.stringify 产生的双引号
        // 与外层 href="..." 双引号冲突导致链接失效（无法返回上一级）。
        parts.push('<a class="bc-link" href="#" data-nav="' + encodeURIComponent(acc) + '" onclick="selectDir(decodeURIComponent(this.dataset.nav)); return false;">' + escapeHtml(s) + '</a>');
      }
    });
  }
  $('breadcrumb').innerHTML = parts.join('');
}

function renderFile(f) {
  // 文件夹：与文件卡片结构一致（复选框、缩略图、文件名、标签、删除）。
  // 注意：文件夹卡不再提供「打开」按钮（点击缩略图/文件名区即可进入）。
  if (f.isDir) {
    const delable = canDel(f);
    return `
      <div class="file-item folder-item">
        <input type="checkbox" class="file-check checkbox" data-name="${escapeAttr(f.name)}" data-del="${delable}" onchange="updateSelectedCount()" onclick="event.stopPropagation()">
        <div class="file-right">
          <div class="file-main" onclick="enterFolder('${escapeAttr(f.name)}')" style="cursor:pointer">
            <div class="thumb-folder">${SVG_FOLDER}</div>
            <div class="file-meta-col">
              <div class="fname fname-row"><span class="ftext">${escapeHtml(f.name)}</span><span class="badge badge-outline">文件夹</span></div>
              <div class="fmeta">${formatSize(f.size)} · ${formatCount(f.count)} 个文件 · ${formatDay(f.mtime)}</div>
            </div>
          </div>
          ${delable ? '<div class="file-actions"><a class="del-btn" data-name="' + escapeAttr(f.name) + '" onclick="event.stopPropagation(); onDelete(this)">删除</a></div>' : ''}
        </div>
      </div>`;
  }
  const url = '/api/download?dir=' + encodeURIComponent(currentDir) + '&file=' + encodeURIComponent(f.name);
  const previewUrl = url + '&inline=1';
  const meta = formatSize(f.size) + ' · ' + formatTime(f.mtime);
  const ext = f.name.split('.').pop().toLowerCase();
  const isImg = IMG_EXTS.includes(ext);
  const isVid = VID_EXTS.includes(ext);
  const previewable = isImg || isVid;
  let thumb;
  if (previewable) {
    if (isImg) {
      // 缩略图走 /api/thumb（服务端缩放），只拉几百字节的小图，避免整张原图卡顿
      const thumbUrl = '/api/thumb?dir=' + encodeURIComponent(currentDir) + '&file=' + encodeURIComponent(f.name) + '&w=240';
      thumb = `<img class="thumb" src="${thumbUrl}" alt="" loading="lazy" decoding="async" onload="this.classList.add('loaded')" data-name="${escapeAttr(f.name)}" style="cursor:pointer" onclick="event.stopPropagation(); openLightboxFromEl(this)">`;
    } else {
      thumb = `<video class="thumb-video" src="${url}" preload="metadata" data-name="${escapeAttr(f.name)}" style="cursor:pointer" onclick="event.stopPropagation(); openLightboxFromEl(this)"></video>`;
    }
  } else {
    thumb = `<div class="thumb-other">${SVG_FILE}</div>`;
  }
  const mainCursor = previewable ? ' style="cursor:pointer"' : '';
  const mainClick = previewable ? ` data-name="${escapeAttr(f.name)}" onclick="openLightboxFromEl(this)"` : '';
  const delable = canDel(f);
  return `
    <div class="file-item">
      <input type="checkbox" class="file-check checkbox" data-name="${escapeAttr(f.name)}" data-del="${delable}" onchange="updateSelectedCount()" onclick="event.stopPropagation()">
      <div class="file-right">
        <div class="file-main"${mainClick}${mainCursor}>
          ${thumb}
          <div class="file-meta-col">
            <div class="fname">${escapeHtml(f.name)}</div>
            <div class="fmeta">${meta}</div>
          </div>
        </div>
        <div class="file-actions">
          <a class="dl-btn" data-url="${url}" data-name="${escapeAttr(f.name)}" onclick="event.stopPropagation(); onSingleDownload(this)">下载</a>
          ${delable ? '<a class="del-btn" data-name="' + escapeAttr(f.name) + '" onclick="event.stopPropagation(); onDelete(this)">删除</a>' : ''}
        </div>
      </div>
    </div>
  `;
}

// ---- 批量下载 ----
function toggleSelectAll(checked, listId) {
  document.querySelectorAll('#' + listId + ' .file-check').forEach(c => { c.checked = checked; });
  updateSelectedCount();
}
function clearChecks(listId) {
  document.querySelectorAll('#' + listId + ' .file-check').forEach(c => { c.checked = false; });
}

function updateSelectedCount() {
  // 文件页：只统计文件列表内的勾选；批量删除仅统计勾选中可删的项
  const nFiles = document.querySelectorAll('#filesList .file-check:checked').length;
  const delFiles = Array.from(document.querySelectorAll('#filesList .file-check:checked')).filter(c => c.dataset.del === 'true').length;
  $('btnSelected').disabled = nFiles === 0;
  $('btnDelSelected').disabled = delFiles === 0;
  // 目录页：只统计目录列表内的勾选；批量删除仅统计勾选中可删的目录
  const nDirs = document.querySelectorAll('#dirsList .file-check:checked').length;
  const delDirs = Array.from(document.querySelectorAll('#dirsList .file-check:checked')).filter(c => c.dataset.del === 'true').length;
  const btnDelDirs = document.getElementById('btnDelDirs');
  if (btnDelDirs) btnDelDirs.disabled = delDirs === 0;
  const btnDownloadDirs = document.getElementById('btnDownloadDirs');
  if (btnDownloadDirs) btnDownloadDirs.disabled = nDirs === 0;
}

// 删除选中的文件：复制「下载选中」的思路，逐个调 /api/delete，成功后即时移除卡片。
// 仅删除可删的项，其余跳过并提示。
async function deleteSelected(btn) {
  const checked = Array.from(document.querySelectorAll('#filesList .file-check:checked'));
  const files = checked.filter(c => c.dataset.del === 'true').map(c => c.dataset.name);
  const skipped = checked.length - files.length;
  if (!files.length) return;
  const desc = '确定删除选中的 ' + files.length + ' 个文件？'
    + (skipped ? '另有 ' + skipped + ' 个来自其他设备、仅可下载，将跳过。' : '')
    + '此操作不可恢复。';
  const ok = await confirmDialog({ title: '删除选中的文件', desc: desc, confirmText: '删除', cancelText: '取消', destructive: true });
  if (!ok) return;
  const btns = document.querySelectorAll('.btn-batch');
  const states = Array.from(btns).map(b => ({ el: b, html: b.innerHTML }));
  btns.forEach(b => { b.disabled = true; });
  let failed = 0;
  try {
    for (const name of files) {
      if (!(await apiDelete(currentDir, name))) failed++;
    }
  } finally {
    btns.forEach(b => { b.classList.remove('loading'); b.innerHTML = states.find(s => s.el === b).html; });
    updateSelectedCount();
    if (failed) {
      selectDir(currentDir); // 部分失败，整表刷新以反映实际状态
    } else {
      // 全部成功：立即移除对应卡片并刷新目录计数，无需整表刷新
      previewFiles = previewFiles.filter(p => !files.includes(p.name));
      files.forEach(name => {
        document.querySelectorAll('.file-item .file-check').forEach(c => {
          if (c.dataset.name === name) { const it = c.closest('.file-item'); if (it) it.remove(); }
        });
      });
      if (!$('filesList').children.length) $('filesEmpty').style.display = 'block';
      loadDirs(false); // 刷新目录页文件计数
    }
  }
}

// 目录页「删除选中目录」：每个勾选项都是顶层目录，删除路径为 ('' , dirName)
// 仅删除可删的目录，其余跳过并提示。
async function deleteSelectedDirs(btn) {
  const checked = Array.from(document.querySelectorAll('#dirsList .file-check:checked'));
  const dirs = checked.filter(c => c.dataset.del === 'true').map(c => c.dataset.name);
  const skipped = checked.length - dirs.length;
  if (!dirs.length) return;
  const desc = '确定删除选中的 ' + dirs.length + ' 个目录及其全部内容？'
    + (skipped ? '另有 ' + skipped + ' 个来自其他设备、仅可下载，将跳过。' : '')
    + '此操作不可恢复。';
  const ok = await confirmDialog({ title: '删除选中的目录', desc: desc, confirmText: '删除', cancelText: '取消', destructive: true });
  if (!ok) return;
  const btns = Array.from(document.querySelectorAll('#dirsList .btn-batch'));
  btns.forEach(b => { b.disabled = true; });
  btn.classList.add('loading');
  btn.innerHTML = '<span class="spinner"></span>删除中...';
  try {
    for (const d of dirs) { await apiDelete('', d); }
  } catch (e) {
    toast('删除失败: ' + e.message, { variant: 'destructive' });
  } finally {
    loadDirs(false);
    updateSelectedCount();
  }
}

// 目录卡片单条「删除」：顶层目录，删除路径为 ('' , dirName)
async function onDeleteDir(btn) {
  const name = btn.dataset.name;
  if (btn.classList.contains('loading')) return;
  const ok = await confirmDialog({ title: '删除目录', desc: '确定删除「' + name + '」及其全部内容？此操作不可恢复。', confirmText: '删除', cancelText: '取消', destructive: true });
  if (!ok) return;
  btn.classList.add('loading');
  const old = btn.textContent;
  btn.textContent = '删除中';
  const delOk = await apiDelete('', name);
  if (!delOk) { btn.classList.remove('loading'); btn.textContent = old; return; }
  const item = btn.closest('.file-item');
  if (item) item.remove();
  updateSelectedCount();
  loadDirs(false);
}

async function downloadSelected(btn) {
  const files = Array.from(document.querySelectorAll('#filesList .file-check:checked')).map(c => c.dataset.name);
  if (!files.length) return;
  await downloadBatch(files, btn);
}

// 目录页批量下载：逐个选中目录打包成 ZIP（files 留空表示整目录），多目录则依次触发下载。
async function downloadSelectedDirs(btn) {
  const dirs = Array.from(document.querySelectorAll('#dirsList .file-check:checked')).map(c => c.dataset.name);
  if (!dirs.length) return;
  const btns = document.querySelectorAll('.btn-batch');
  btns.forEach(b => { b.disabled = true; });
  btn.classList.add('loading');
  btn.innerHTML = '<span class="spinner"></span>下载中...';
  try {
    for (const dir of dirs) {
      const res = await fetch('/api/download-batch', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ dir: dir, files: [] }),
      });
      if (!res.ok) { toast('下载失败: ' + res.status, { variant: 'destructive' }); continue; }
      const blob = await res.blob();
      downloadBlob(blob, (dir.split('/').pop() || 'dir') + '.zip');
    }
  } catch (e) {
    toast('下载失败: ' + e.message, { variant: 'destructive' });
  } finally {
    btn.classList.remove('loading');
    btn.textContent = '下载';
    updateSelectedCount();
  }
}

// 单文件下载：fetch + blob，下载完成在 finally 立即恢复按钮，精确感知不靠定时器猜
async function onSingleDownload(btn) {
  if (btn.classList.contains('loading')) return;
  btn.classList.add('loading');
  btn.innerHTML = '<span class="spinner"></span>下载中';
  try {
    const res = await fetch(btn.dataset.url);
    if (!res.ok) { toast('下载失败: ' + res.status, { variant: 'destructive' }); return; }
    const blob = await res.blob();
    downloadBlob(blob, btn.dataset.name);
  } catch (e) {
    toast('下载失败: ' + e.message, { variant: 'destructive' });
  } finally {
    btn.classList.remove('loading');
    btn.textContent = '下载';
  }
}

// 单文件删除：确认后调 /api/delete，成功后刷新文件列表（重建 previewFiles 与索引）
async function onDelete(btn) {
  const name = btn.dataset.name;
  if (btn.classList.contains('loading')) return;
  const ok = await confirmDialog({ title: '删除文件', desc: '确定删除「' + name + '」？此操作不可恢复。', confirmText: '删除', cancelText: '取消', destructive: true });
  if (!ok) return;
  btn.classList.add('loading');
  const old = btn.textContent;
  btn.textContent = '删除中';
  const delOk = await apiDelete(currentDir, name);
  if (!delOk) {
    btn.classList.remove('loading');
    btn.textContent = old;
    return;
  }
  // 成功后立即从界面移除该卡片（无需整表刷新）
  const item = btn.closest('.file-item');
  if (item) item.remove();
  previewFiles = previewFiles.filter(p => p.name !== name);
  updateSelectedCount();
  if (!$('filesList').children.length) $('filesEmpty').style.display = 'block';
  loadDirs(false); // 刷新目录页文件计数
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
    if (!res.ok) { toast('下载失败: ' + res.status, { variant: 'destructive' }); return; }
    const blob = await res.blob();
    const zipBase = currentDir.split('/').pop() || 'files';
    downloadBlob(blob, zipBase + '.zip');
  } catch (e) {
    toast('下载失败: ' + e.message, { variant: 'destructive' });
  } finally {
    btns.forEach(b => { b.classList.remove('loading'); b.innerHTML = states.find(s => s.el === b).html; });
    updateSelectedCount();
  }
}

// ---- 灯箱预览：左右切换（PC 按钮 / 移动端滑动）+ 键盘导航 ----
function openLightbox(idx) {
  if (idx < 0 || idx >= previewFiles.length) return;
  lbIndex = idx;
  showLbImage();
  $('lightbox').style.display = 'flex';
  lockScroll(); // 锁 html+body，背景不可滚动（html 是实际滚动容器，只锁 body 无效）
  const lb = $('lightbox');
  lb.addEventListener('touchstart', lbTouchStart, { passive: true });
  lb.addEventListener('touchmove', lbTouchMove, { passive: false });
  lb.addEventListener('touchend', lbTouchEnd, { passive: true });
  document.addEventListener('keydown', lbKeyHandler);
}

// 按文件名打开灯箱：点击时实时在 previewFiles 中查找索引，
// 避免删除文件后其余卡片仍持有失效的旧 idx（删除后 previewFiles 已被过滤、索引错位）。
function openLightboxFromEl(el) {
  const name = el.dataset.name;
  const i = previewFiles.findIndex(p => p.name === name);
  if (i >= 0) openLightbox(i);
}

// 让底部工具条宽度等于当前显示的图片/视频宽度（图片尺寸动态，须用 JS 同步）
function syncBarWidth() {
  const bar = $('lbBar');
  if (!bar || $('lightbox').style.display !== 'flex') return;
  const lbImg = $('lbImg');
  const lbVideo = $('lbVideo');
  let w = 0;
  if (lbImg.style.display !== 'none' && lbImg.offsetWidth > 0) w = lbImg.offsetWidth;
  else if (lbVideo.style.display !== 'none' && lbVideo.offsetWidth > 0) w = lbVideo.offsetWidth;
  if (w > 0) bar.style.width = w + 'px';
}
window.addEventListener('resize', syncBarWidth);

function closeLightbox() {
  $('lightbox').style.display = 'none';
  $('lbVideo').pause();
  unlockScroll();
  const lb = $('lightbox');
  lb.removeEventListener('touchstart', lbTouchStart);
  lb.removeEventListener('touchmove', lbTouchMove);
  lb.removeEventListener('touchend', lbTouchEnd);
  document.removeEventListener('keydown', lbKeyHandler);
  lbTouchStartPt = null;
}

// ---- 灯箱触摸滑动切换（移动端；桌面端仍用左右箭头） ----
let lbTouchStartPt = null;
function lbTouchStart(e) {
  if (e.touches.length !== 1) return;
  // 点到按钮（关闭/下载/删除）不触发滑动
  if (e.target.closest && e.target.closest('button')) return;
  lbTouchStartPt = { x: e.touches[0].clientX, y: e.touches[0].clientY };
}
function lbTouchMove(e) {
  if (!lbTouchStartPt || e.touches.length !== 1) return;
  const dx = e.touches[0].clientX - lbTouchStartPt.x;
  const dy = e.touches[0].clientY - lbTouchStartPt.y;
  // 横向主导时阻止浏览器默认手势（下拉刷新/橡皮筋），视频控件区域除外（避免破坏进度条拖动）
  if (Math.abs(dx) > Math.abs(dy) && !(e.target.closest && e.target.closest('video'))) {
    e.preventDefault();
  }
}
function lbTouchEnd(e) {
  if (!lbTouchStartPt || e.changedTouches.length !== 1) return;
  const t = e.changedTouches[0];
  const dx = t.clientX - lbTouchStartPt.x;
  const dy = t.clientY - lbTouchStartPt.y;
  const startY = lbTouchStartPt.y;
  lbTouchStartPt = null;
  if (previewFiles.length < 2) return;              // 仅 1 个文件不切换
  if (Math.abs(dx) < 60 || Math.abs(dx) < Math.abs(dy)) return; // 位移不够或竖向手势
  // 视频：触摸起点在底部进度条区域（~72px）不触发，避免拖动进度条误切
  if ($('lbVideo').style.display !== 'none' && startY > window.innerHeight - 72) return;
  lbNav(dx < 0 ? 1 : -1);
}

function lbNav(delta) {
  lbIndex = (lbIndex + delta + previewFiles.length) % previewFiles.length;
  showLbImage();
}

// 预加载相邻图片（仅图片），翻页时直接走浏览器缓存，消除加载闪烁
function preloadNeighbors() {
  if (previewFiles.length < 2) return;
  [lbIndex - 1, lbIndex + 1].forEach(i => {
    const p = previewFiles[(i + previewFiles.length) % previewFiles.length];
    if (p && !VID_EXTS.includes(p.ext)) {
      const im = new Image();
      im.src = p.previewUrl;
    }
  });
}

function showLbImage() {
  const f = previewFiles[lbIndex];
  const isVid = VID_EXTS.includes(f.ext);
  const lbImg = $('lbImg');
  const lbVideo = $('lbVideo');
  lbImg.style.display = isVid ? 'none' : 'block';
  lbVideo.style.display = isVid ? 'block' : 'none';
  if (isVid) {
    lbVideo.src = f.previewUrl;
  } else {
    const newSrc = f.previewUrl;
    if (lbImg.getAttribute('src') === newSrc) {
      lbImg.classList.add('loaded');             // 同一张（如重新打开），直接显示，避免卡在透明态
    } else {
      lbImg.classList.remove('loaded');         // 先淡出，加载完成再淡入（消除翻页闪动）
      lbImg.onload = () => { lbImg.classList.add('loaded'); syncBarWidth(); };
      lbImg.src = newSrc;
    }
    preloadNeighbors();                          // 预加载相邻图片，左右翻页秒出
  }
  // 视频/图片加载完成后同步（顶部已无底栏，syncBarWidth 内部空函数安全返回）
  lbVideo.onloadedmetadata = syncBarWidth;
  syncBarWidth();
  // 仅 1 个文件时隐藏左右箭头
  const showNav = previewFiles.length > 1;
  $('lbPrev').style.display = showNav ? 'flex' : 'none';
  $('lbNext').style.display = showNav ? 'flex' : 'none';
  // 重置顶部下载按钮为图标态
  const dlBtn = $('lbDownload');
  dlBtn.classList.remove('loading');
  dlBtn.disabled = false;
  dlBtn.innerHTML = SVG_DOWNLOAD;
  // 删除按钮：仅本设备上传的文件可删（切换图片时同步显隐）
  $('lbDelete').style.display = canDel(f) ? '' : 'none';
}

// 灯箱内下载当前文件：fetch + blob，下载中禁用按钮显示 loading
async function lbDownload() {
  const btn = $('lbDownload');
  if (btn.classList.contains('loading')) return;
  const f = previewFiles[lbIndex];
  const url = f.previewUrl.replace('&inline=1', '');
  btn.classList.add('loading');
  btn.disabled = true;
  btn.innerHTML = '<span class="spinner"></span>';
  try {
    const res = await fetch(url);
    if (!res.ok) { toast('下载失败: ' + res.status, { variant: 'destructive' }); return; }
    const blob = await res.blob();
    downloadBlob(blob, f.name);
  } catch (e) {
    toast('下载失败: ' + e.message, { variant: 'destructive' });
  } finally {
    btn.classList.remove('loading');
    btn.disabled = false;
    btn.innerHTML = SVG_DOWNLOAD;
  }
}

// 灯箱内删除当前文件：确认后调 /api/delete，成功后自动跳到下一张继续预览（不再退回目录）
async function lbDelete() {
  if (lbIndex < 0 || lbIndex >= previewFiles.length) return;
  const f = previewFiles[lbIndex];
  if (!f) return;
  const ok = await confirmDialog({ title: '删除文件', desc: '确定删除「' + f.name + '」？此操作不可恢复。', confirmText: '删除', cancelText: '取消', destructive: true });
  if (!ok) return;
  const okDel = await apiDelete(currentDir, f.name);
  if (!okDel) return;
  // 从 DOM 列表移除对应卡片，保持与预览数组同步
  document.querySelectorAll('.file-item .file-check').forEach(c => {
    if (c.dataset.name === f.name) { const it = c.closest('.file-item'); if (it) it.remove(); }
  });
  previewFiles = previewFiles.filter(p => p.name !== f.name);
  updateSelectedCount();
  if (!$('filesList').children.length) $('filesEmpty').style.display = 'block';
  loadDirs(false); // 刷新目录页文件计数

  // 已无剩余文件则关闭灯箱，否则自动定位到下一张（被删项后的元素左移填补原索引；删的是末张则退回上一张）
  if (previewFiles.length === 0) {
    closeLightbox();
    return;
  }
  if (lbIndex >= previewFiles.length) lbIndex = previewFiles.length - 1;
  showLbImage();
}

function lbKeyHandler(e) {
  if (e.key === 'ArrowLeft') lbNav(-1);
  else if (e.key === 'ArrowRight') lbNav(1);
  else if (e.key === 'Escape') closeLightbox();
}

async function backToDirs() { sessionStorage.removeItem('aellus_currentDir'); await loadDirs(false); show('dirs'); }

function formatSize(b) {
  if (b < 1024) return b + ' B';
  if (b < 1048576) return (b/1024).toFixed(1) + ' KB';
  if (b < 104857600) return (b/1048576).toFixed(2) + ' MB'; // < 100MB → MB
  return (b/1073741824).toFixed(2) + ' GB'; // ≥ 100MB → GB
}
// 文件数：超过 1 万则换算成 “X.X 万”
function formatCount(n) {
  if (n >= 10000) return (n/10000).toFixed(1) + '万';
  return String(n);
}
// 仅日期，隐藏时分：2026-08-16
function formatDay(ts) {
  const d = new Date(ts * 1000);
  const p = n => String(n).padStart(2,'0');
  return `${d.getFullYear()}-${p(d.getMonth()+1)}-${p(d.getDate())}`;
}
// 文件卡片时间：日期 + 时分
function formatTime(ts) {
  const d = new Date(ts * 1000);
  const p = n => String(n).padStart(2,'0');
  return formatDay(ts) + ' ' + p(d.getHours()) + ':' + p(d.getMinutes());
}
function escapeAttr(s) { return s.replace(/"/g, '&quot;'); }

// 公共：触发浏览器下载一个 blob（创建临时 <a> → click → 释放 URL）
function downloadBlob(blob, filename) {
  const a = document.createElement('a');
  a.href = URL.createObjectURL(blob);
  a.download = filename;
  a.click();
  URL.revokeObjectURL(a.href);
}

// 公共：调用 /api/delete，成功返回 true，失败弹 toast 并返回 false
async function apiDelete(dir, name) {
  try {
    const res = await fetch('/api/delete', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ dir: dir, file: name }),
    });
    if (!res.ok) {
      const d = await res.json().catch(() => ({}));
      toast('删除失败: ' + (d.error || res.status), { variant: 'destructive' });
      return false;
    }
    return true;
  } catch (e) {
    toast('删除失败: ' + e.message, { variant: 'destructive' });
    return false;
  }
}

loadDirs();

// 滚动吸顶毛玻璃：未滚动时 nav 与 batch-bar 分开、无背景；一旦下滑即整条满宽模糊
const topBars = document.querySelectorAll('.top-bar');
function syncTopBarBlur() {
  const scrolled = window.scrollY > 0;
  topBars.forEach(b => b.classList.toggle('scrolled', scrolled));
}
window.addEventListener('scroll', syncTopBarBlur, { passive: true });
syncTopBarBlur();
