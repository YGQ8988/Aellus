// 首页访问地址栏：展示当前服务地址 + 一键复制。
// 局域网多为 http（非 secure context），navigator.clipboard 不可用，故用 execCommand 兜底。
(function () {
  var urlEl = document.getElementById('accessUrl');
  var btn = document.getElementById('copyBtn');
  if (!urlEl || !btn) return;

  var origin = window.location.origin;
  urlEl.textContent = origin;

  // 自适应缩放：URL 一行显示全。从 14px 逐步降到 9px，直到 scrollWidth ≤ 容器宽度；
  // 仍放不下则保留横向滚动兜底。resize 时重算。
  function fitUrl() {
    for (var s = 14; s >= 9; s--) {
      urlEl.style.fontSize = s + 'px';
      if (urlEl.scrollWidth <= urlEl.clientWidth) break;
    }
  }
  fitUrl();
  window.addEventListener('resize', fitUrl);

  var copyIco = btn.querySelector('.ico-copy');
  var checkIco = btn.querySelector('.ico-check');

  function setCopied(on) {
    if (copyIco) copyIco.style.display = on ? 'none' : '';
    if (checkIco) checkIco.style.display = on ? '' : 'none';
    btn.classList.toggle('copied', on);
  }

  // 优先用 Clipboard API（需 secure context，如 https / localhost）；
  // 否则降级到临时 textarea + execCommand，兼容局域网 http 访问。
  function copyText(text) {
    if (navigator.clipboard && window.isSecureContext) {
      return navigator.clipboard.writeText(text);
    }
    return new Promise(function (resolve, reject) {
      var ta = document.createElement('textarea');
      ta.value = text;
      ta.setAttribute('readonly', '');
      ta.style.position = 'fixed';
      ta.style.left = '-9999px';
      document.body.appendChild(ta);
      ta.select();
      var ok = false;
      try { ok = document.execCommand('copy'); } catch (e) {}
      document.body.removeChild(ta);
      ok ? resolve() : reject();
    });
  }

  function doCopy() {
    copyText(origin).then(function () {
      setCopied(true);
      setTimeout(function () { setCopied(false); }, 1500);
    });
  }

  btn.addEventListener('click', doCopy);
  // 点击 URL 文本同样复制
  urlEl.addEventListener('click', doCopy);
  urlEl.style.cursor = 'pointer';

  // ---- 二维码弹窗 ----
  var qrBtn = document.getElementById('qrBtn');
  var qrModal = document.getElementById('qrModal');
  var qrBackdrop = document.getElementById('qrBackdrop');
  var qrClose = document.getElementById('qrClose');
  var qrImg = document.getElementById('qrImg');
  var qrUrlText = document.getElementById('qrUrlText');

  function showQR() {
    qrImg.innerHTML = '';
    qrUrlText.textContent = origin;
    try {
      // qrcode-generator 库：typeNumber=0 自动选最小版本，纠错级别 'M'
      var qr = qrcode(0, 'M');
      qr.addData(origin);
      qr.make();
      // scalable SVG，自适应容器尺寸；margin 留白便于扫描
      qrImg.innerHTML = qr.createSvgTag({ cellSize: 0, margin: 0, scalable: true });
    } catch (e) {
      qrImg.innerHTML = '<div style="color:var(--red);font-size:13px;">二维码生成失败</div>';
    }
    qrModal.classList.add('open');
    document.body.style.overflow = 'hidden';
  }

  function hideQR() {
    qrModal.classList.remove('open');
    document.body.style.overflow = '';
  }

  if (qrBtn && qrModal) {
    qrBtn.addEventListener('click', showQR);
    qrClose.addEventListener('click', hideQR);
    qrBackdrop.addEventListener('click', hideQR);
    document.addEventListener('keydown', function (e) { if (e.key === 'Escape') hideQR(); });
  }
})();

// ---- 信息弹窗（点击图标）：本机显示存储目录，所有人显示赞赏 ----
(function () {
  var icon = document.getElementById('appIcon');
  var modal = document.getElementById('infoModal');
  if (!icon || !modal) return;
  var backdrop = document.getElementById('infoBackdrop');
  var closeBtn = document.getElementById('infoClose');
  var dirSection = document.getElementById('saveDirSection');
  var dirCurrent = document.getElementById('saveDirCurrent');
  var dirInput = document.getElementById('saveDirInput');
  var dirApply = document.getElementById('saveDirApply');
  var dirReset = document.getElementById('saveDirReset');
  var dirMsg = document.getElementById('saveDirMsg');
  var defaultDir = '';

  function showMsg(text, ok) {
    dirMsg.textContent = text;
    dirMsg.className = 'savedir-msg ' + (ok ? 'ok' : 'err');
  }

  function setDir(dir) {
    dirCurrent.textContent = dir;
    dirInput.value = '';
    dirInput.placeholder = dir;
  }

  function openInfo() {
    modal.classList.add('open');
    document.body.style.overflow = 'hidden';
    dirSection.style.display = 'none'; // 先隐藏，本机 fetch 成功才展开
    // 尝试获取存储目录：本机返回数据则显示目录区块，非本机 403 则保持隐藏（只显示赞赏）
    fetch('/api/savedir').then(function (res) { return res.json(); }).then(function (data) {
      if (data && data.dir) {
        setDir(data.dir);
        defaultDir = data.default || '';
        dirSection.style.display = '';
      }
    }).catch(function () {});
  }

  function closeInfo() {
    modal.classList.remove('open');
    document.body.style.overflow = '';
    showMsg('', true);
  }

  icon.addEventListener('click', openInfo);
  icon.addEventListener('keydown', function (e) { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); openInfo(); } });
  closeBtn.addEventListener('click', closeInfo);
  backdrop.addEventListener('click', closeInfo);
  document.addEventListener('keydown', function (e) { if (e.key === 'Escape') closeInfo(); });

  // 底部署名栏也可点击打开信息弹窗（不影响 Star 链接）
  var authorTip = document.getElementById('authorTip');
  if (authorTip) {
    authorTip.style.cursor = 'pointer';
    authorTip.addEventListener('click', function (e) {
      if (e.target.closest('.star-tip')) return; // 点 Star 区域不弹窗，让链接正常跳转
      openInfo();
    });
  }

  // 修改存储目录
  function submitDir(dir, done) {
    fetch('/api/setsavedir', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ dir: dir })
    }).then(function (res) { return res.json(); }).then(function (data) {
      if (data && data.ok) { setDir(data.dir); showMsg(done || '已修改', true); }
      else { showMsg((data && data.error) || '修改失败', false); }
    }).catch(function () { showMsg('网络错误', false); });
  }

  dirApply.addEventListener('click', function () {
    var dir = dirInput.value.trim();
    if (!dir) { showMsg('请输入目录路径', false); return; }
    dirApply.disabled = true;
    submitDir(dir, '已修改');
    dirApply.disabled = false;
  });

  // 恢复默认：直接提交默认目录
  dirReset.addEventListener('click', function () {
    if (!defaultDir) { showMsg('未获取到默认目录', false); return; }
    dirReset.disabled = true;
    submitDir(defaultDir, '已恢复默认目录');
    dirReset.disabled = false;
  });
})();
