// 页面基准路径：静态资源与接口都用相对路径（static/... 、api/...），解析依赖 <base>。
// 服务端把注入前缀写在 <html data-base>：裸端口直连为 "/"，飞牛统一网关为 "/app/<appname>/"
//（网关转发可能已剥离前缀、地址栏可能无尾斜杠，前端无法从 location 推断）。
// 本文件在 <head> 内同步加载（须早于其它资源引用），三个页面共用这一份。
(function () {
  'use strict';
  var base = document.documentElement.dataset.base || '/';
  try {
    var p = location.pathname || '/';
    var pre = base;
    while (pre.length > 1 && pre.charAt(pre.length - 1) === '/') pre = pre.slice(0, -1);
    // 地址栏与注入前缀一致（可能仅大小写不同，如 /app/Aellus）时，沿用地址栏的真实写法。
    if (pre && pre !== '/' && p.slice(0, pre.length).toLowerCase() === pre.toLowerCase()) {
      base = p.slice(0, pre.length) + '/';
    }
  } catch (e) {}
  var b = document.createElement('base');
  b.href = base;
  document.head.appendChild(b);
})();
