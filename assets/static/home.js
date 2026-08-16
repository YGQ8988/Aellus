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
