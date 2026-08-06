// 读取页逻辑：选目录 → 列文件 → 下载
const $ = id => document.getElementById(id);
let currentDir = '';

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
        <div class="dicon">📁</div>
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
  let thumb, preview = '';
  if (['png','jpg','jpeg','gif','webp','bmp','heic'].includes(ext)) {
    thumb = `<img class="thumb" src="${url}" alt="" loading="lazy">`;
    preview = `<a class="preview-link" href="${previewUrl}" target="_blank">预览</a>`;
  } else if (['mp4','mov','m4v','webm'].includes(ext)) {
    thumb = `<video class="thumb-video" src="${url}" preload="metadata"></video>`;
    preview = `<a class="preview-link" href="${previewUrl}" target="_blank">播放</a>`;
  } else {
    thumb = `<div class="thumb-other">📄</div>`;
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

async function downloadAll() { await downloadBatch([]); }

async function downloadSelected() {
  const files = Array.from(document.querySelectorAll('.file-check:checked')).map(c => c.dataset.name);
  if (!files.length) return;
  await downloadBatch(files);
}

async function downloadBatch(files) {
  const btn = $('btnSelected');
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
  }
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
