// 读取页逻辑：选目录 → 列文件 → 下载 / 预览
//（$ / formatSize / formatDay / formatTime 等通用工具来自 ui.js 共享层）
const IMG_EXTS = ['png','jpg','jpeg','gif','webp','bmp','heic'];
const VID_EXTS = ['mp4','mov','m4v','webm'];

// 图标 SVG（跨平台渲染一致）
const SVG_FOLDER  = '<svg class="icon" viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"/></svg>';
const SVG_FILE    = '<svg class="icon" viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/></svg>';
const SVG_DOWNLOAD = '<svg class="icon" viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>';

let currentDir = '';
let allDirs = []; // 最近一次 /api/dirs 返回的目录列表，用于决定面包屑是否显示「目录」层级
const PAGE_SIZE = 30;   // 每页条数（列表不足 2 页时不显示分页器）
let filesPage = 1;      // 文件列表：当前页 / 总数（分页器与空态判定用）
let filesTotal = 0;
let dirsPage = 1;       // 文件夹列表：当前页 / 总数
let dirsTotal = 0;
let previewFiles = []; // [{name, previewUrl, ext}, ...] 当前目录可预览文件
let lbIndex = 0;       // 灯箱当前索引
// 是否有删除权限（含删目录 / 批量删）：服务端按「桌面端本机 / 飞牛门户内」算好，随列表响应
// 的顶层 canDelete 下发——同一请求内所有条目一致（见 Go 端 canManage）。渲染列表前更新；
// 前端判定仅用于按钮显隐，真正的强制在服务端。
let canManage = false;

function show(view) {
  $('dirsView').classList.toggle('active', view === 'dirs');
  $('filesView').classList.toggle('active', view === 'files');
  // 切换视图时清空另一视图的勾选，避免跨视图污染批量操作
  clearChecks(view === 'dirs' ? 'filesList' : 'dirsList');
  updateSelectedCount();
  if (typeof syncTopBarBlur === 'function') syncTopBarBlur();
  if (typeof syncToTop === 'function') syncToTop();
}

// ---- 通用页码分页器（文件夹列表 / 文件列表共用）----
// 不足 2 页（total ≤ size）时不显示；onGo(page) 由调用方提供（一般：回顶部 + 拉取该页）。
function renderPager(el, total, page, size, onGo) {
  el.innerHTML = '';
  const pages = Math.ceil(total / size);
  if (pages <= 1) { el.hidden = true; return; }
  el.hidden = false;
  const info = document.createElement('span');
  info.className = 'pager-total';
  info.textContent = '共 ' + total + ' 个';
  el.appendChild(info);
  function btn(label, act, cls) {
    const b = document.createElement('button');
    b.type = 'button';
    b.className = 'pager-btn' + (cls ? ' ' + cls : '');
    b.textContent = label;
    if (typeof act === 'number') b.onclick = () => onGo(act);
    else b.disabled = true;
    el.appendChild(b);
  }
  btn('‹', page > 1 ? page - 1 : null);
  // 页码窗口：≤7 页全列；否则 1 … (当前±1) … 末页
  const seq = [];
  if (pages <= 7) {
    for (let i = 1; i <= pages; i++) seq.push(i);
  } else {
    seq.push(1);
    if (page > 3) seq.push('…');
    for (let i = Math.max(2, page - 1); i <= Math.min(pages - 1, page + 1); i++) seq.push(i);
    if (page < pages - 2) seq.push('…');
    seq.push(pages);
  }
  seq.forEach(it => {
    if (it === '…') {
      const s = document.createElement('span');
      s.className = 'pager-ellipsis';
      s.textContent = '…';
      el.appendChild(s);
    } else if (it === page) {
      btn(String(it), null, 'active');
    } else {
      btn(String(it), it);
    }
  });
  btn('›', page < pages ? page + 1 : null);
}

// ---- 加载文件夹列表（分页：每页 PAGE_SIZE 条，超过 1 页时列表下方出现页码分页器）----
async function loadDirs(autoEnter = true, page) {
  const pg = page || 1;
  $('dirsLoading').style.display = 'block';
  $('dirsList').innerHTML = '';
  $('dirsEmpty').style.display = 'none';
  $('dirsPager').hidden = true;
  try {
    const res = await fetch('api/dirs?page=' + pg + '&size=' + PAGE_SIZE);
    const data = await res.json();
    allDirs = data.dirs;
    dirsPage = data.page || 1;
    dirsTotal = data.total || 0;
    $('dirsLoading').style.display = 'none';
    if (!data.total) {
      $('dirsEmpty').style.display = 'block';
      return;
    }
    if (!data.dirs.length) {
      // 本页为空但总数不为 0（页码越界，如删除后）→ 回退到最后一页
      const last = Math.ceil(data.total / PAGE_SIZE);
      if (pg > last) { loadDirs(false, last); return; }
    }
    canManage = !!data.canDelete;
    $('dirsList').innerHTML = data.dirs.map(d => {
      const delable = canManage;
      return `
      <div class="file-item dir-card">
        <input type="checkbox" class="file-check checkbox" data-name="${escapeAttr(d.name)}" data-del="${delable}" onchange="updateSelectedCount()" onclick="event.stopPropagation()">
        <div class="file-right">
          <div class="file-main" onclick="selectDir(${jsLit(d.name)})" style="cursor:pointer">
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
    // 无删除权限（局域网设备直连）时直接隐藏批量删除按钮，不显示置灰态
    $('btnDelDirs').style.display = data.canDelete ? '' : 'none';
    updateSelectedCount();
    // 页码分页器：不足 2 页自动隐藏
    renderPager($('dirsPager'), dirsTotal, dirsPage, PAGE_SIZE, p => { window.scrollTo(0, 0); loadDirs(false, p); });
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
      //（用 total 判定：分页下 data.dirs 可能只是第一页）
      if (target === null && data.total === 1 && data.dirs.length === 1) {
        target = data.dirs[0].name;
      }
    }
    if (target !== null) selectDir(target);
  } catch (e) {
    $('dirsLoading').textContent = '加载失败: ' + e.message;
  }
}

// ---- 打开某个目录（支持子目录路径，如 "设备名/子目录"；与文件夹列表共用同一套页码分页器）----
async function openDir(path, page) {
  currentDir = path;
  const pg = page || 1;
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
  $('filesPager').hidden = true;
  try {
    const res = await fetch('api/files?dir=' + encodeURIComponent(path) + '&page=' + pg + '&size=' + PAGE_SIZE);
    const data = await res.json();
    $('filesLoading').style.display = 'none';
    $('filesList').classList.remove('swapping');
    // data.error 也做 HTML 转义：服务端当前只返回固定文案，但这里是 innerHTML sink，
    // 一旦将来把用户可控内容（如目录名）拼进错误信息就会变成 XSS。
    if (data.error) { $('filesList').innerHTML = '<div class="empty">' + escapeHtml(data.error) + '</div>'; buildBreadcrumb(); return; }
    filesPage = data.page || 1;
    filesTotal = data.total || 0;
    if (!data.total) { previewFiles = []; $('filesList').innerHTML = ''; $('filesEmpty').style.display = 'block'; buildBreadcrumb(); return; }
    if (!data.files.length) {
      // 本页为空但目录有内容（页码越界，如删除后）→ 回退到最后一页
      const last = Math.ceil(data.total / PAGE_SIZE);
      if (pg > last) { openDir(path, last); return; }
    }
    // 构建可预览文件列表（图片 + 视频），供灯箱左右切换（文件夹不进预览）
    previewFiles = data.files.filter(f => {
      if (f.isDir) return false;
      const ext = f.name.split('.').pop().toLowerCase();
      return IMG_EXTS.includes(ext) || VID_EXTS.includes(ext);
    }).map(f => {
      const u = 'api/download?dir=' + encodeURIComponent(path) + '&file=' + encodeURIComponent(f.name);
      return { name: f.name, previewUrl: u + '&inline=1', ext: f.name.split('.').pop().toLowerCase() };
    });
    canManage = !!data.canDelete;
    $('filesList').innerHTML = data.files.map(renderFile).join('');
    // 显示批量操作栏，重置选中状态
    $('batchBar').style.display = 'flex';
    $('selectAll').checked = false;
    // 无删除权限（局域网设备直连）时直接隐藏批量删除按钮，不显示置灰态
    $('btnDelSelected').style.display = data.canDelete ? '' : 'none';
    updateSelectedCount();
    buildBreadcrumb();
    // 页码分页器：不足 2 页自动隐藏
    renderPager($('filesPager'), filesTotal, filesPage, PAGE_SIZE, p => { window.scrollTo(0, 0); openDir(currentDir, p); });
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
  // 返回首页必须用相对路径 "./"：它会按页面 <base> 解析——
  //   飞牛门户内（base=/app/<appname>/）→ 回应用首页；
  //   局域网直连（base=/）→ 回站点首页。
  // 写死 "/" 会跑到站点根：门户内即飞牛桌面（iframe 跳出应用），局域网下也会丢应用前缀。
  const parts = ['<a class="bc-link" href="./" onclick="clearAellusDir()">← 返回首页</a>'];
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
    const delable = canManage;
    return `
      <div class="file-item folder-item">
        <input type="checkbox" class="file-check checkbox" data-name="${escapeAttr(f.name)}" data-del="${delable}" onchange="updateSelectedCount()" onclick="event.stopPropagation()">
        <div class="file-right">
          <div class="file-main" onclick="enterFolder(${jsLit(f.name)})" style="cursor:pointer">
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
  const url = 'api/download?dir=' + encodeURIComponent(currentDir) + '&file=' + encodeURIComponent(f.name);
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
      const thumbUrl = 'api/thumb?dir=' + encodeURIComponent(currentDir) + '&file=' + encodeURIComponent(f.name) + '&w=240';
      thumb = `<img class="thumb" src="${thumbUrl}" alt="" loading="lazy" decoding="async" onload="this.classList.add('loaded')" data-name="${escapeAttr(f.name)}" style="cursor:pointer" onclick="event.stopPropagation(); openLightboxFromEl(this)">`;
    } else {
      thumb = `<video class="thumb-video" src="${url}" preload="metadata" data-name="${escapeAttr(f.name)}" style="cursor:pointer" onclick="event.stopPropagation(); openLightboxFromEl(this)"></video>`;
    }
  } else {
    thumb = `<div class="thumb-other">${SVG_FILE}</div>`;
  }
  const mainCursor = previewable ? ' style="cursor:pointer"' : '';
  const mainClick = previewable ? ` data-name="${escapeAttr(f.name)}" onclick="openLightboxFromEl(this)"` : '';
  const delable = canManage;
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
  try {
    for (const name of files) {
      await apiDelete(currentDir, name);   // 失败已在 apiDelete 内 toast
    }
  } finally {
    btns.forEach(b => { b.classList.remove('loading'); b.innerHTML = states.find(s => s.el === b).html; });
    updateSelectedCount();
    // 分页下删除会让后续页错位，统一重拉当前页以反映实际状态；
    // 本页被删空时 openDir 会自动回退到最后一页。
    loadDirs(false);                       // 刷新文件夹列表的文件计数
    openDir(currentDir, filesPage);
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
    loadDirs(false, dirsPage);   // 分页下重拉当前页（删空自动回退）
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
  updateSelectedCount();
  loadDirs(false, dirsPage);   // 重拉当前页（分页下保证准确；删空自动回退）
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
      const blob = await fetchBlob('api/download-batch', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ dir: dir, files: [] }),
      });
      if (blob) downloadBlob(blob, (dir.split('/').pop() || 'dir') + '.zip');
    }
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
    const blob = await fetchBlob(btn.dataset.url);
    if (blob) downloadBlob(blob, btn.dataset.name);
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
  // 成功后重拉当前页（分页下保证数据准确；本页删空会自动回退到最后一页）
  loadDirs(false);                       // 刷新文件夹列表的文件计数
  openDir(currentDir, filesPage);
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
    const blob = await fetchBlob('api/download-batch', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ dir: currentDir, files: files }),
    });
    if (blob) {
      const zipBase = currentDir.split('/').pop() || 'files';
      downloadBlob(blob, zipBase + '.zip');
    }
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
  // 清掉平移切换可能的中途残留（幽灵层 + 主图内联样式），保证下次打开是干净状态
  document.querySelectorAll('.lb-ghost').forEach(g => g.remove());
  const li = $('lbImg');
  li.style.transition = ''; li.style.transform = ''; li.style.opacity = '';
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
  showLbImage(delta);
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

// dir：切换方向（+1 下一张 / -1 上一张；不传 = 无方向：首开、删除补位等直接淡入）。
// 前后都是图片且带方向时走「平移切换」（旧图滑出 + 新图滑入），其余场景保持淡入。
function showLbImage(dir) {
  const f = previewFiles[lbIndex];
  const isVid = VID_EXTS.includes(f.ext);
  const lbImg = $('lbImg');
  const lbVideo = $('lbVideo');
  const wasImgShown = lbImg.style.display !== 'none' && !!lbImg.getAttribute('src');
  lbImg.style.display = isVid ? 'none' : 'block';
  lbVideo.style.display = isVid ? 'block' : 'none';
  if (isVid) {
    lbVideo.src = f.previewUrl;
  } else {
    // 防御：上一次平移若被打断（如滑入中途关闭灯箱），内联样式可能残留
    //（内联优先级高于 .loaded，残留会让图偏移/透明）→ 先清掉再走各分支。
    lbImg.style.transition = '';
    lbImg.style.transform = '';
    lbImg.style.opacity = '';
    const newSrc = f.previewUrl;
    if (dir && wasImgShown && lbImg.getAttribute('src') !== newSrc) {
      slideToImage(dir, newSrc);                 // 平移切换：旧图滑出 + 新图滑入
      preloadNeighbors();
    } else if (lbImg.getAttribute('src') === newSrc) {
      lbImg.classList.add('loaded');             // 同一张（如重新打开），直接显示，避免卡在透明态
    } else {
      lbImg.classList.remove('loaded');         // 先淡出，加载完成再淡入（消除翻页闪动）
      lbImg.onload = () => { lbImg.classList.add('loaded'); syncBarWidth(); };
      lbImg.src = newSrc;
      preloadNeighbors();                        // 预加载相邻图片，左右翻页秒出
    }
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
  // 删除按钮：无管理权限（局域网设备直连）时隐藏（切换图片时同步显隐）
  $('lbDelete').style.display = canManage ? '' : 'none';
}

// ---- 灯箱「平移切换」 ----
// 时长；CSS 里 .lb-ghost 的过渡参数与此保持一致（LB_SLIDE_MS）。
const LB_SLIDE_MS = 260;
// 位移 = 图片宽度的 12%，clamp 在 48~120px：小图不过冲、大图也有明显的平移感。
const LB_SLIDE_RATIO = 0.12;
const LB_SLIDE_MIN = 48;
const LB_SLIDE_MAX = 120;

// 把当前图定格为幽灵层（.lb-ghost）向反方向滑出并淡出，新图从 dir 方向滑入
//（+1 = 新图从右侧进入，-1 = 从左侧）。灯箱大图居中且四周留黑，整屏滑动会有
// 空档感，小幅平移（按图宽比例）+ 渐隐更接近相册翻页的手感。
let lbSlideSeq = 0;                            // 平移切换代次：仅最新一次动画的收尾逻辑生效
function slideToImage(dir, newSrc) {
  const seq = ++lbSlideSeq;
  const lbImg = $('lbImg');
  const lb = $('lightbox');
  // 1) 定格当前图：按实际显示位置/尺寸复制一份（fixed 矩形），随后滑出
  const r = lbImg.getBoundingClientRect();
  const shift = Math.max(LB_SLIDE_MIN, Math.min(LB_SLIDE_MAX, Math.round(r.width * LB_SLIDE_RATIO)));
  const ghost = document.createElement('img');
  ghost.className = 'lb-ghost';
  ghost.src = lbImg.getAttribute('src');
  ghost.style.left = r.left + 'px';
  ghost.style.top = r.top + 'px';
  ghost.style.width = r.width + 'px';
  ghost.style.height = r.height + 'px';
  lb.appendChild(ghost);
  // 2) 主图瞬时置于滑入起点（不可见），下一帧起动画（双 rAF 保证起点已上屏）
  lbImg.onload = null;                           // 滑入期间不需要 onload 兜底（结束统一补 loaded）
  lbImg.classList.remove('loaded');
  lbImg.style.transition = 'none';
  lbImg.style.transform = 'translateX(' + (dir * shift) + 'px)';
  lbImg.style.opacity = '0';
  lbImg.src = newSrc;
  requestAnimationFrame(() => requestAnimationFrame(() => {
    ghost.style.transform = 'translateX(' + (-dir * shift) + 'px)';
    ghost.style.opacity = '0';
    lbImg.style.transition = 'transform ' + LB_SLIDE_MS + 'ms ease, opacity ' + LB_SLIDE_MS + 'ms ease';
    lbImg.style.transform = 'translateX(0)';
    lbImg.style.opacity = '1';
  }));
  // 3) 幽灵层移除（transitionend + 超时双保险）
  const dropGhost = () => ghost.remove();
  ghost.addEventListener('transitionend', dropGhost, { once: true });
  setTimeout(dropGhost, LB_SLIDE_MS + 120);
  // 4) 主图收尾：清内联、回 .loaded 淡入态。用代次（seq）保证只有最新一次动画的
  //    收尾生效；transitionend 在动画被打断（如滑入中途关闭灯箱）时不触发，
  //    故再加一道超时兜底，避免内联样式残留导致下次打开时图偏移/透明。
  const finishSlide = () => {
    lbImg.style.transition = '';
    lbImg.style.transform = '';
    lbImg.style.opacity = '';
    lbImg.classList.add('loaded');
  };
  lbImg.addEventListener('transitionend', function onSlideEnd(ev) {
    if (ev.target !== lbImg || ev.propertyName !== 'transform') return;
    lbImg.removeEventListener('transitionend', onSlideEnd);
    if (seq === lbSlideSeq) finishSlide();
  });
  setTimeout(() => { if (seq === lbSlideSeq) finishSlide(); }, LB_SLIDE_MS + 150);
}

// 灯箱内下载当前文件：fetch + blob，下载中禁用按钮显示 loading
async function lbDownload() {
  const btn = $('lbDownload');
  if (btn.classList.contains('loading')) return;
  const f = previewFiles[lbIndex];
  btn.classList.add('loading');
  btn.disabled = true;
  btn.innerHTML = '<span class="spinner"></span>';
  try {
    const blob = await fetchBlob(f.previewUrl.replace('&inline=1', ''));
    if (blob) downloadBlob(blob, f.name);
  } finally {
    btn.classList.remove('loading');
    btn.disabled = false;
    btn.innerHTML = SVG_DOWNLOAD;
  }
}

// 灯箱内删除当前文件：确认后调 /api/delete；成功后重拉当前页——
// 列表自动补位（第 2 页首个滑入本页，与卡片删除同一机制）、分页器与计数刷新
//（本页删空会自动回退到最后一页）；再在新一轮预览列表里定位“原下一张”继续预览，
// 没有可预览项则关闭灯箱。
async function lbDelete() {
  if (lbIndex < 0 || lbIndex >= previewFiles.length) return;
  const f = previewFiles[lbIndex];
  if (!f) return;
  const ok = await confirmDialog({ title: '删除文件', desc: '确定删除「' + f.name + '」？此操作不可恢复。', confirmText: '删除', cancelText: '取消', destructive: true });
  if (!ok) return;
  // 删除前记录继续预览的目标（优先下一张，其次上一张）：重拉后按名字定位
  const nextName = (previewFiles[lbIndex + 1] || previewFiles[lbIndex - 1] || {}).name || null;
  const okDel = await apiDelete(currentDir, f.name);
  if (!okDel) return;
  await openDir(currentDir, filesPage);
  loadDirs(false); // 刷新文件夹列表的文件计数
  // 在新预览列表里定位继续预览的图片；找不到（目录已空）则关闭灯箱
  const idx = nextName ? previewFiles.findIndex(p => p.name === nextName) : -1;
  if (idx < 0) { closeLightbox(); return; }
  lbIndex = idx;
  showLbImage();
}

function lbKeyHandler(e) {
  if (e.key === 'ArrowLeft') lbNav(-1);
  else if (e.key === 'ArrowRight') lbNav(1);
  else if (e.key === 'Escape') closeLightbox();
}

async function backToDirs() { sessionStorage.removeItem('aellus_currentDir'); await loadDirs(false); show('dirs'); }

// 文件数：超过 1 万则换算成 “X.X 万”
function formatCount(n) {
  if (n >= 10000) return (n/10000).toFixed(1) + '万';
  return String(n);
}
// 注：formatSize / formatDay / formatTime 已统一到 ui.js 共享层（全站同一实现）
function escapeAttr(s) { return s.replace(/"/g, '&quot;'); }
// jsLit：生成可安全放进 HTML 内联事件处理器（onclick="..."）里的 JS 字符串字面量。
// 先用 JSON.stringify 做 JS 层转义（处理 ' " \ 及控制字符），再把 " 转成 &quot; 适配外层双引号属性。
// 仅用于 onclick="fn(${jsLit(x)})" 这类「把用户数据作为 JS 字符串参数」的场景；
// 普通属性值（如 data-name="..."）仍用 escapeAttr 即可。
function jsLit(s) {
  return JSON.stringify(String(s == null ? '' : s)).replace(/&/g, '&amp;').replace(/"/g, '&quot;');
}

// 下载类请求的统一前置：fetch → 非 ok / 网络异常时弹 toast 并返回 null，
// 正常返回 blob。调用方只需判空，错误提示与 try/catch 不再各处重复。
async function fetchBlob(url, init) {
  try {
    const res = await fetch(url, init);
    if (!res.ok) { toast('下载失败: ' + res.status, { variant: 'destructive' }); return null; }
    return await res.blob();
  } catch (e) {
    toast('下载失败: ' + e.message, { variant: 'destructive' });
    return null;
  }
}

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
    const res = await fetch('api/delete', {
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
// 返回顶部：滚动超过一屏的少量距离后浮现（点击平滑回顶）
function scrollToTop() {
  window.scrollTo({ top: 0, behavior: 'smooth' });
}
function syncToTop() {
  $('toTop').classList.toggle('show', window.scrollY > 320);
}
window.addEventListener('scroll', function () { syncTopBarBlur(); syncToTop(); }, { passive: true });
syncTopBarBlur();
syncToTop();
