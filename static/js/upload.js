// 上传页逻辑
const $ = id => document.getElementById(id);

// 与读取页一致的字段格式化（大小 / 时间），保证上传成功卡片与 files-grid 卡片字段统一
function formatSize(b) {
  if (b < 1024) return b + ' B';
  if (b < 1048576) return (b/1024).toFixed(1) + ' KB';
  if (b < 1073741824) return (b/1048576).toFixed(2) + ' MB';      // < 1024 MB
  if (b < 1099511627776) return (b/1073741824).toFixed(2) + ' GB'; // < 1024 GB
  return (b/1099511627776).toFixed(2) + ' TB';
}
// 上传速度格式化：始终以 MB/s 显示（不上 GB/s，避免单位跳变且局域网内极少突破 1GB/s）
function formatSpeed(bps) {
  const mbps = bps / 1048576;
  if (mbps < 0.1) return (mbps * 1000).toFixed(0) + ' KB/s';
  return mbps.toFixed(1) + ' MB/s';
}
function formatTime(ts) {
  const d = new Date(ts * 1000);
  const p = n => String(n).padStart(2,'0');
  const day = `${d.getFullYear()}-${p(d.getMonth()+1)}-${p(d.getDate())}`;
  return day + ' ' + p(d.getHours()) + ':' + p(d.getMinutes());
}

function getFiles(input) {
  if (!input.files.length) return;
  $('device').value = $('device').value.trim();
  upload(Array.from(input.files), input);
  input.value = '';
}

// 取文件在上传时的“逻辑名”：优先用拖文件夹时手动记录的相对路径，
// 其次用 <input webkitdirectory> 的 webkitRelativePath，最后退回纯文件名。
function fileUploadName(f) {
  return f.relPath || f.webkitRelativePath || f.name;
}

// 拍照/录像：优先用网页内相机（getUserMedia），失败（非安全上下文或不支持）时回退到系统选择器
let camStream = null;
let mediaRecorder = null;
let recordedChunks = [];
let camMode = 'photo';

function hasCameraAPI() {
  return !!(navigator.mediaDevices && navigator.mediaDevices.getUserMedia) && window.isSecureContext;
}

async function openCamera(mode) {
  camMode = mode;
  if (!hasCameraAPI()) {
    // 兼容性无法统一：HTTPS 下可直接调相机；非安全上下文(HTTP)浏览器不支持，
    // 这里回退到系统文件选择器，交给浏览器自身决定（部分浏览器仍可直接拍照）
    $('camStatus').textContent = '';
    (mode === 'video' ? $('recInput') : $('camInput')).click();
    return;
  }
  try {
    camStream = await navigator.mediaDevices.getUserMedia({
      video: { facingMode: 'environment' },
      audio: mode === 'video'
    });
  } catch (e) {
    $('camStatus').textContent = '⚠️ 无法访问摄像头：' + e.message + '（请确认用 https 打开并已授权）';
    return;
  }
  $('camVideo').srcObject = camStream;
  $('camPreview').style.display = 'none';
  $('camPreview').innerHTML = '';
  $('camVideo').style.display = 'block';
  $('camConfirm').style.display = 'none';
  $('camCapture').style.display = camMode === 'photo' ? '' : 'none';
  $('camRecord').style.display = camMode === 'video' ? '' : 'none';
  $('camRecord').textContent = '开始录像';
  $('camStatus').textContent = '';
  $('camModal').style.display = 'flex';
}

function closeCamera() {
  if (camStream) {
    camStream.getTracks().forEach(t => t.stop());
    camStream = null;
  }
  mediaRecorder = null;
  $('camModal').style.display = 'none';
}

$('camCancel').onclick = closeCamera;

// 拍照：把当前视频帧截到 canvas，导出 jpeg
$('camCapture').onclick = () => {
  const v = $('camVideo');
  if (!v.videoWidth) return;
  const canvas = document.createElement('canvas');
  canvas.width = v.videoWidth;
  canvas.height = v.videoHeight;
  canvas.getContext('2d').drawImage(v, 0, 0);
  canvas.toBlob(blob => {
    const file = new File([blob], 'aellus_photo_' + Date.now() + '.jpg', { type: 'image/jpeg' });
    $('camVideo').style.display = 'none';
    const img = document.createElement('img');
    img.src = URL.createObjectURL(blob);
    const box = $('camPreview');
    box.innerHTML = '';
    box.appendChild(img);
    box.style.display = 'block';
    $('camCapture').style.display = 'none';
    $('camConfirm').style.display = '';
    $('camConfirm').onclick = () => { upload([file], null); closeCamera(); };
  }, 'image/jpeg', 0.9);
};

function pickMime() {
  const cands = ['video/webm;codecs=vp9,opus', 'video/webm;codecs=vp8,opus', 'video/webm', 'video/mp4'];
  for (const c of cands) {
    if (window.MediaRecorder && MediaRecorder.isTypeSupported(c)) return c;
  }
  return '';
}

// 录像：MediaRecorder 录制，停止后导出（安卓多为 webm，少数支持 mp4）
$('camRecord').onclick = () => {
  if (!mediaRecorder) {
    const mime = pickMime();
    try {
      mediaRecorder = new MediaRecorder(camStream, mime ? { mimeType: mime } : undefined);
    } catch (e) {
      $('camStatus').textContent = '⚠️ 当前浏览器不支持录像：' + e.message;
      return;
    }
    recordedChunks = [];
    mediaRecorder.ondataavailable = e => { if (e.data && e.data.size) recordedChunks.push(e.data); };
    mediaRecorder.onstop = () => {
      const type = mediaRecorder.mimeType || 'video/webm';
      const blob = new Blob(recordedChunks, { type });
      const ext = type.indexOf('mp4') >= 0 ? 'mp4' : 'webm';
      const file = new File([blob], 'aellus_video_' + Date.now() + '.' + ext, { type });
      $('camVideo').style.display = 'none';
      const pv = document.createElement('video');
      pv.src = URL.createObjectURL(blob);
      pv.controls = true;
      const box = $('camPreview');
      box.innerHTML = '';
      box.appendChild(pv);
      box.style.display = 'block';
      $('camRecord').style.display = 'none';
      $('camConfirm').style.display = '';
      $('camConfirm').onclick = () => { upload([file], null); closeCamera(); };
    };
    mediaRecorder.start();
    $('camRecord').textContent = '停止录像';
    $('camStatus').textContent = '● 录制中...';
  } else {
    mediaRecorder.stop();
    mediaRecorder = null;
    $('camStatus').textContent = '';
  }
};

['fileInput','folderInput','fileInputPhoto','camInput','recInput'].forEach(id => {
  $(id).addEventListener('change', () => getFiles($(id)));
});

// 拖放上传：主卡片拖放区支持拖入文件 / 文件夹；整块虚线区均可点击选文件
(function () {
  const dropArea = document.getElementById('dropArea');
  const uploadZone = document.getElementById('uploadZone');
  // 整个上传区（虚线框）点击即触发普通文件选择，不必只点“点击选择”文字
  dropArea.addEventListener('click', () => {
    const fi = document.getElementById('fileInput');
    if (fi) fi.click();
  });
  ['dragenter','dragover'].forEach(evt => {
    dropArea.addEventListener(evt, e => {
      e.preventDefault(); e.stopPropagation();
      uploadZone.classList.add('upload-zone--active');
    }, false);
  });
  ['dragleave','drop'].forEach(evt => {
    dropArea.addEventListener(evt, e => {
      e.preventDefault(); e.stopPropagation();
      uploadZone.classList.remove('upload-zone--active');
    }, false);
  });
  dropArea.addEventListener('drop', e => {
    e.preventDefault(); e.stopPropagation();
    uploadZone.classList.remove('upload-zone--active');

    // 优先用 File System Access 拖放 API 递归展开文件夹（拖入文件夹可上传其全部内容）
    const items = e.dataTransfer.items;
    if (items && items.length && items[0].webkitGetAsEntry) {
      const entries = [];
      for (const it of items) {
        const getAsEntry = it.webkitGetAsEntry || it.mozGetAsEntry;
        const entry = getAsEntry ? getAsEntry.call(it) : null;
        if (entry) entries.push(entry);
      }
      if (entries.length) {
        collectDroppedFiles(entries, files => {
          if (files.length) upload(files, null);
        });
        return;
      }
    }

    // 回退：普通多文件（不支持 entry API 的环境）
    const files = [];
    if (items) {
      for (const item of items) {
        if (item.kind === 'file') {
          const f = item.getAsFile();
          if (f) files.push(f);
        }
      }
    } else {
      for (const f of e.dataTransfer.files) files.push(f);
    }
    if (files.length) upload(files, null);
  }, false);
})();

// 拖入文件夹时，用 webkitGetAsEntry 递归收集目录内所有真实文件（目录本身不上传）
function traverseEntry(entry, basePath, out, done) {
  const full = basePath ? basePath + '/' + entry.name : entry.name;
  if (entry.isFile) {
    entry.file(f => { try { f.relPath = full; } catch (e) {} out.push(f); done(); }, done);
  } else if (entry.isDirectory) {
    const reader = entry.createReader();
    const readBatch = () => {
      reader.readEntries(batch => {
        if (!batch.length) { done(); return; }
        let pending = batch.length;
        batch.forEach(b => traverseEntry(b, full, out, () => {
          pending--;
          if (pending === 0) readBatch();
        }));
      }, done);
    };
    readBatch();
  } else {
    done();
  }
}

function collectDroppedFiles(entries, done) {
  const out = [];
  let pending = entries.length;
  if (pending === 0) { done(out); return; }
  entries.forEach(entry => traverseEntry(entry, '', out, () => {
    pending--;
    if (pending === 0) done(out);
  }));
}

function upload(files, inputEl) {
  if (!files || !files.length) return;
  const fd = new FormData();
  // 注意：multipart 的 filename 会被服务端/库清洗成纯文件名(丢掉目录)，
  // 所以相对路径(含层级)必须放在独立的 rels 字段，与 files 索引一一对应。
  files.forEach(f => {
    fd.append('files', f);
    fd.append('rels', fileUploadName(f));
  });
  fd.append('device', $('device').value.trim() || 'default');

  const xhr = new XMLHttpRequest();
  const prog = $('progress');
  prog.style.display = 'block';
  prog.innerHTML = '';
  $('result').style.display = 'none';

  // 每个文件一行进度卡片（结构：上排缩略图/文件名/速度 + 取消按钮，下排独立进度条）
  const rows = files.map(f => {
    const item = document.createElement('div');
    item.className = 'up-prog-item';

    const row = document.createElement('div');
    row.className = 'up-prog-row';

    const main = document.createElement('div');
    main.className = 'file-main';

    // 左侧缩略图 / 文件图标
    if (f.type.startsWith('image/')) {
      const img = document.createElement('img');
      img.className = 'thumb';
      img.alt = '预览';
      const reader = new FileReader();
      reader.onload = () => { img.src = reader.result; img.classList.add('loaded'); };
      reader.readAsDataURL(f);
      main.appendChild(img);
    } else if (f.type.startsWith('video/')) {
      const v = document.createElement('video');
      v.className = 'thumb-video';
      v.muted = true; v.preload = 'metadata';
      v.src = URL.createObjectURL(f);
      main.appendChild(v);
    } else {
      const ph = document.createElement('div');
      ph.className = 'thumb-other';
      // 取文件后缀名（大写）作为图标；无后缀名时才回退到通用文件 SVG 图标
      const ext = (f.name.indexOf('.') >= 0) ? f.name.split('.').pop().toUpperCase() : '';
      if (ext) {
        ph.classList.add('thumb-ext');
        ph.textContent = ext.slice(0, 4);
      } else {
        ph.innerHTML = '<svg class="icon" viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"></path><polyline points="14 2 14 8 20 8"></polyline></svg>';
      }
      main.appendChild(ph);
    }

    const metaCol = document.createElement('div');
    metaCol.className = 'file-meta-col';
    const fname = document.createElement('div');
    fname.className = 'fname';
    fname.textContent = f.name;
    const fmeta = document.createElement('div');
    fmeta.className = 'fmeta';
    fmeta.textContent = '准备中…';
    metaCol.appendChild(fname);
    metaCol.appendChild(fmeta);
    main.appendChild(metaCol);

    // 右侧取消按钮
    const cancelBtn = document.createElement('button');
    cancelBtn.type = 'button';
    cancelBtn.className = 'up-prog-cancel';
    cancelBtn.setAttribute('aria-label', '取消上传');
    cancelBtn.innerHTML = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>';

    row.appendChild(main);
    row.appendChild(cancelBtn);

    // 下排：独立进度条
    const bar = document.createElement('div');
    bar.className = 'up-prog-bar';
    const fill = document.createElement('div');
    fill.className = 'up-prog-bar-fill';
    bar.appendChild(fill);

    item.appendChild(row);
    item.appendChild(bar);
    prog.appendChild(item);
    return { fmeta, fill, item, cancelBtn, f };
  });

  // 取消上传：单 xhr 整批共享，点任意行的叉号 → 整批 abort，所有行变 cancelled 状态后淡出
  const doCancel = () => {
    if (xhr.readyState !== 0 && xhr.readyState !== 4) {
      try { xhr.abort(); } catch (e) {}
    }
    rows.forEach(r => {
      r.item.classList.add('is-cancelled');
      r.cancelBtn.disabled = true;
      r.fmeta.textContent = '已取消';
    });
  };
  rows.forEach(r => { r.cancelBtn.onclick = doCancel; });

  // 速度计算：记录起始时间与已上报字节
  const t0 = performance.now();
  let lastLoaded = 0, lastT = t0;

  xhr.upload.onprogress = e => {
    if (!e.lengthComputable) return;
    const now = performance.now();
    const pct = (e.loaded / e.total * 100);
    // 整体瞬时速度（基于最近一次采样间隔）
    const dt = (now - lastT) / 1000;
    let speed = '';
    if (dt > 0.2) {
      const bps = (e.loaded - lastLoaded) / dt;
      speed = formatSpeed(bps);
      lastLoaded = e.loaded; lastT = now;
    }
    rows.forEach(r => {
      const p = Math.max(0, Math.min(100, pct));
      r.fmeta.textContent = (speed ? speed + ' · ' : '') + p.toFixed(0) + '%';
      r.fill.style.width = p + '%';
    });
  };
  xhr.onload = () => {
    if (xhr.status === 200) {
      const res = JSON.parse(xhr.responseText);
      prog.style.display = 'none';
      showResult(res, files);
    } else {
      prog.style.display = 'none';
      const box = $('result');
      box.style.display = 'block';
      let msg = '上传失败: ' + xhr.statusText;
      try {
        const res = JSON.parse(xhr.responseText);
        if (res && res.message) msg = '上传失败：' + res.message;
      } catch (e) {}
      box.innerHTML = '';
      const al = document.createElement('div'); al.className = 'alert alert-destructive';
      const at = document.createElement('div'); at.className = 'alert-title'; at.textContent = '上传失败';
      const ad = document.createElement('div'); ad.className = 'alert-desc'; ad.textContent = msg;
      al.appendChild(at); al.appendChild(ad); box.appendChild(al);
    }
  };
  xhr.onerror = () => {
    if (xhr.readyState === 4 && xhr.status === 0) {
      // 主动 abort 触发：不弹错误，由 doCancel 的 cancelled 状态接管
      return;
    }
    prog.style.display = 'none';
    const box = $('result');
    box.style.display = 'block';
    box.innerHTML = '';
    const al = document.createElement('div'); al.className = 'alert alert-destructive';
    const at = document.createElement('div'); at.className = 'alert-title'; at.textContent = '网络错误';
    const ad = document.createElement('div'); ad.className = 'alert-desc'; ad.textContent = '上传失败，请检查网络连接后重试。';
    al.appendChild(at); al.appendChild(ad); box.appendChild(al);
  };
  xhr.open('POST', '/upload');
  xhr.send(fd);
  if (inputEl) inputEl.value = '';
}

function showResult(res, files) {
  const box = $('result');
  box.style.display = 'block';
  box.innerHTML = '';

  const head = document.createElement('div');
  head.className = 'result-head';
  head.textContent = '已保存到：' + res.dir;
  box.appendChild(head);

  const grid = document.createElement('div');
  grid.className = 'up-grid';
  res.files.forEach((f, i) => {
    const card = document.createElement('div');
    card.className = 'up-card';

    const nameEl = document.createElement('div');
    nameEl.className = 'up-name';
    nameEl.title = f.name;
    nameEl.textContent = f.name;

    const thumb = document.createElement('div');
    thumb.className = 'up-thumb';
    if (files[i] && files[i].type.startsWith('image/')) {
      const img = document.createElement('img');
      img.alt = '预览';
      const reader = new FileReader();
      reader.onload = () => { img.src = reader.result; };
      reader.readAsDataURL(files[i]);
      thumb.appendChild(img);
    } else {
      const ph = document.createElement('div');
      ph.className = 'up-thumb-ph';
      ph.textContent = (f.name.split('.').pop() || 'file').toUpperCase().slice(0, 4);
      thumb.appendChild(ph);
    }

    const sizeEl = document.createElement('div');
    sizeEl.className = 'up-size';
    const time = f.mtime ? formatTime(f.mtime) : '';
    sizeEl.textContent = formatSize(f.size) + (time ? ' · ' + time : '');

    const meta = document.createElement('div');
    meta.className = 'up-meta';
    meta.appendChild(nameEl);
    meta.appendChild(sizeEl);

    card.appendChild(thumb);
    card.appendChild(meta);
    grid.appendChild(card);
  });
  box.appendChild(grid);
}
