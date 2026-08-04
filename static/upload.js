// 上传页逻辑
const $ = id => document.getElementById(id);
const deviceKey = 'droplan-device';
$('device').value = localStorage.getItem(deviceKey) || '';

function getFiles(input) {
  if (!input.files.length) return;
  $('device').value = $('device').value.trim() || 'default';
  localStorage.setItem(deviceKey, $('device').value);
  upload(Array.from(input.files), input);
  input.value = '';
}

['fileInput','camInput','recInput'].forEach(id => {
  $(id).addEventListener('change', () => getFiles($(id)));
});

function upload(files, inputEl) {
  const fd = new FormData();
  files.forEach(f => fd.append('files', f, f.name));
  fd.append('device', $('device').value);

  const xhr = new XMLHttpRequest();
  const prog = $('progress'), bar = prog.firstElementChild;
  prog.style.display = 'block';
  $('status').textContent = '上传中...';
  $('result').style.display = 'none';

  xhr.upload.onprogress = e => {
    if (e.lengthComputable) bar.style.width = (e.loaded / e.total * 100) + '%';
  };
  xhr.onload = () => {
    if (xhr.status === 200) {
      const res = JSON.parse(xhr.responseText);
      prog.style.display = 'none';
      $('status').textContent = '✅ ' + files.length + ' 个文件已上传到电脑';
      showResult(res, files);
    } else {
      $('status').textContent = '❌ 上传失败: ' + xhr.statusText;
      prog.style.display = 'none';
    }
  };
  xhr.onerror = () => {
    $('status').textContent = '❌ 网络错误';
    prog.style.display = 'none';
  };
  xhr.open('POST', '/upload');
  xhr.send(fd);
}

function showResult(res, files) {
  const box = $('result');
  box.style.display = 'block';
  box.innerHTML = '<div style="font-size:13px;color:var(--text-2);">已保存到：' + res.dir + '</div>';
  res.files.forEach((f, i) => {
    const div = document.createElement('div');
    div.style.marginTop = '10px';
    let html = '<div class="fname">' + f.name + '</div><div class="fsize">' + (f.size/1048576).toFixed(2) + ' MB</div>';
    if (files[i] && files[i].type.startsWith('image/')) {
      html += '<img src="data:;base64,' + '" alt="预览">';
      const reader = new FileReader();
      reader.onload = () => { div.querySelector('img').src = reader.result; };
      reader.readAsDataURL(files[i]);
    }
    div.innerHTML = html;
    box.appendChild(div);
  });
}
