// 作者署名轮播：三行循环切换，每 3 秒一条，带淡入动画。
// 三页通用（首页/上传页/浏览页都引入）。
(function () {
  var lines = [
    'The Power From AI!',
    'The Design By Yang Guangqing!',
    'The Style By Yang Junwen!'
  ];
  var idx = 0;
  var el = document.getElementById('authorTip');
  if (!el) return;
  var span = el.querySelector('.author-text');
  if (!span) return;
  span.textContent = lines[0];
  setInterval(function () {
    idx = (idx + 1) % lines.length;
    span.textContent = lines[idx];
    // 触发淡入动画：移除再加回 class，强制重播
    span.classList.remove('fade');
    void span.offsetWidth;
    span.classList.add('fade');
  }, 3000);
})();
