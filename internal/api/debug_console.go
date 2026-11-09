package api

import (
	"net/http"
)

// debugConsole 提供一个内置的接口调试页，方便在浏览器里直接验证 API。
func (s *Server) debugConsole(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(debugConsoleHTML))
}

const debugConsoleHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>SonicHub API 调试台</title>
<style>
  :root { --bg:#0f1115; --panel:#171a21; --line:#262b36; --text:#e6e9ef; --muted:#8b93a7; --accent:#4f8cff; --ok:#3ecf8e; --warn:#ffb454; --bad:#ff6b6b; }
  * { box-sizing: border-box; }
  body { margin:0; padding:24px; font-family: ui-sans-serif, -apple-system, "Segoe UI", "Microsoft YaHei", sans-serif; background:var(--bg); color:var(--text); }
  h1 { font-size:20px; margin:0 0 4px; }
  .sub { color:var(--muted); font-size:13px; margin-bottom:18px; }
  .sub code { color:var(--text); }
  .grid { display:grid; grid-template-columns: 380px 1fr; gap:16px; align-items:start; }
  .col { display:flex; flex-direction:column; gap:16px; }
  .card { background:var(--panel); border:1px solid var(--line); border-radius:10px; padding:16px; }
  .card h2 { font-size:14px; margin:0 0 12px; color:var(--muted); font-weight:600; letter-spacing:.4px; }
  label { display:block; font-size:12px; color:var(--muted); margin:10px 0 4px; }
  input, textarea, select { width:100%; background:#0d1015; color:var(--text); border:1px solid var(--line); border-radius:6px; padding:8px 10px; font-size:13px; font-family:inherit; }
  textarea { min-height:88px; resize:vertical; font-family: ui-monospace, Consolas, monospace; }
  button { background:var(--accent); color:#fff; border:0; border-radius:6px; padding:9px 14px; font-size:13px; cursor:pointer; }
  button:hover { filter:brightness(1.1); }
  button:disabled { opacity:.45; cursor:not-allowed; }
  button.ghost { background:transparent; border:1px solid var(--line); color:var(--text); }
  .row { display:flex; gap:8px; margin-top:12px; flex-wrap:wrap; }
  .status { margin-top:12px; font-size:12px; color:var(--muted); }
  .presets { display:flex; flex-wrap:wrap; gap:6px; margin-top:6px; }
  .presets button { font-size:12px; padding:6px 10px; background:#202632; border:1px solid var(--line); color:var(--text); }
  pre { background:#0d1015; border:1px solid var(--line); border-radius:8px; padding:12px; min-height:240px; max-height:50vh; overflow:auto; margin:0; font-size:12.5px; line-height:1.55; white-space:pre-wrap; word-break:break-all; }
  .meta { font-size:12px; color:var(--muted); margin-bottom:8px; }
  .meta .code-ok { color:var(--ok); } .meta .code-bad { color:var(--bad); }
  .wiz-grid { display:grid; grid-template-columns: repeat(3, minmax(0,1fr)); gap:0 12px; }
  .wiz-grid .full { grid-column: 1 / -1; }
  .check { display:flex; align-items:center; gap:6px; margin-top:20px; font-size:12px; color:var(--muted); }
  .check input { width:auto; }
  .steps { margin-top:14px; display:flex; flex-direction:column; gap:8px; }
  .step { display:flex; align-items:center; gap:10px; font-size:13px; padding:8px 10px; background:#0d1015; border:1px solid var(--line); border-radius:8px; }
  .step .dot { width:8px; height:8px; border-radius:50%; background:#39404d; flex:none; }
  .step .st { margin-left:auto; font-size:12px; color:var(--muted); }
  .step.run .dot { background:var(--warn); box-shadow:0 0 0 3px rgba(255,180,84,.15); }
  .step.run .st { color:var(--warn); }
  .step.done .dot { background:var(--ok); }
  .step.done .st { color:var(--ok); }
  .step.fail .dot { background:var(--bad); }
  .step.fail .st { color:var(--bad); }
  .progress { height:6px; background:#0d1015; border:1px solid var(--line); border-radius:99px; overflow:hidden; margin-top:12px; }
  .bar { height:100%; width:0; background:linear-gradient(90deg,#4f8cff,#3ecf8e); transition:width .35s ease; }
  audio { width:100%; margin-top:12px; display:none; }
  @media (max-width: 980px) { .grid { grid-template-columns: 1fr; } .wiz-grid { grid-template-columns: repeat(2, minmax(0,1fr)); } }
</style>
</head>
<body>
<h1>SonicHub API 调试台</h1>
<div class="sub">演示账号 <code>13800138000</code> / 验证码 <code>1234</code>　·　服务地址 <code id="origin"></code></div>

<div class="grid">
  <div class="col">
    <div class="card">
      <h2>1. 登录</h2>
      <label>手机号</label>
      <input id="phone" value="13800138000">
      <label>验证码</label>
      <input id="code" value="1234">
      <div class="row"><button id="loginBtn">登录并保存 Token</button></div>
      <div class="status" id="authStatus">未登录</div>
    </div>

    <div class="card">
      <h2>2. 自由请求</h2>
      <label>方法 + 路径</label>
      <div style="display:flex; gap:8px">
        <select id="method" style="width:96px">
          <option>GET</option><option>POST</option><option>PUT</option><option>DELETE</option>
        </select>
        <input id="path" value="/api/v1/feed/recommend">
      </div>
      <label>请求体（JSON，GET 可留空）</label>
      <textarea id="body"></textarea>
      <div class="row">
        <button id="sendBtn">发送请求</button>
        <button class="ghost" id="clearBtn">清空输出</button>
      </div>
      <label>快捷示例</label>
      <div class="presets" id="presets"></div>
    </div>
  </div>

  <div class="col">
    <div class="card">
      <h2>3. 写歌全链路：报价 → 生成 → 进度 → 发布</h2>
      <div class="wiz-grid">
        <div>
          <label>风格（逗号分隔，最多 3 个）</label>
          <input id="wizStyles" value="电子,Lo-fi">
        </div>
        <div>
          <label>时长（秒）</label>
          <select id="wizDuration">
            <option>15</option><option selected>30</option><option>60</option><option>90</option><option>120</option>
          </select>
        </div>
        <div>
          <label>音质</label>
          <select id="wizQuality">
            <option>标准</option><option selected>高清</option>
          </select>
        </div>
        <div>
          <label>标题</label>
          <input id="wizTitle" value="雨停之后">
        </div>
        <div>
          <label>人声</label>
          <select id="wizVocal">
            <option>女声</option><option>男声</option><option>纯音乐</option>
          </select>
        </div>
        <div class="check"><input type="checkbox" id="wizStems"><label for="wizStems" style="margin:0">需要分轨</label></div>
        <div class="full">
          <label>创作描述（prompt，必填，≤500 字）</label>
          <textarea id="wizPrompt">雨后的城市，路灯把水洼照亮，副歌温暖上扬</textarea>
        </div>
        <div class="check full"><input type="checkbox" id="wizAutoPublish" checked><label for="wizAutoPublish" style="margin:0">生成完成后自动发布（不勾选可手动点「③ 发布作品」）</label></div>
      </div>

      <div class="row">
        <button id="runAll">一键跑完整链路</button>
        <button class="ghost" id="btnQuote">① 报价</button>
        <button class="ghost" id="btnGen">② 生成</button>
        <button class="ghost" id="btnPublish">③ 发布作品</button>
      </div>

      <div class="steps">
        <div class="step" id="step-quote"><span class="dot"></span><span>① 报价</span><span class="st">待执行</span></div>
        <div class="step" id="step-generate"><span class="dot"></span><span>② 创建生成任务</span><span class="st">待执行</span></div>
        <div class="step" id="step-progress"><span class="dot"></span><span>③ 生成进度</span><span class="st">待执行</span></div>
        <div class="step" id="step-publish"><span class="dot"></span><span>④ 发布作品</span><span class="st">待执行</span></div>
      </div>
      <div class="progress"><div class="bar" id="bar"></div></div>
      <div class="meta" id="wizMeta" style="margin-top:10px">尚未开始</div>
      <audio id="player" controls></audio>
    </div>

    <div class="card">
      <h2>响应</h2>
      <div class="meta" id="respMeta">等待发送…</div>
      <pre id="output"></pre>
    </div>
  </div>
</div>

<script>
(function () {
  var TOKEN_KEY = 'sonichub_debug_token';
  var token = localStorage.getItem(TOKEN_KEY) || '';
  var origin = location.origin;
  document.getElementById('origin').textContent = origin;

  var authStatus = document.getElementById('authStatus');
  var output = document.getElementById('output');
  var respMeta = document.getElementById('respMeta');
  var wizMeta = document.getElementById('wizMeta');
  var bar = document.getElementById('bar');
  var player = document.getElementById('player');

  var wiz = { quoteId: '', genId: '', workId: '', total: 0, watching: false };

  function pretty(text) {
    try { return JSON.stringify(JSON.parse(text), null, 2); } catch (e) { return text; }
  }

  function showResp(method, path, status, text, ms) {
    var cls = status >= 200 && status < 300 ? 'code-ok' : 'code-bad';
    respMeta.innerHTML = method + ' ' + path + '　<span class="' + cls + '">HTTP ' + status + '</span>' + (ms != null ? '　' + ms + 'ms' : '');
    output.textContent = pretty(text);
  }

  function setStep(key, state, text) {
    var el = document.getElementById('step-' + key);
    el.className = 'step' + (state ? ' ' + state : '');
    el.querySelector('.st').textContent = text;
  }

  function resetSteps() {
    ['quote', 'generate', 'progress', 'publish'].forEach(function (k) { setStep(k, '', '待执行'); });
    bar.style.width = '0%';
    player.style.display = 'none';
    player.removeAttribute('src');
    wiz.quoteId = ''; wiz.genId = ''; wiz.workId = ''; wiz.total = 0;
    wizMeta.textContent = '尚未开始';
  }

  function refreshAuthStatus() {
    authStatus.textContent = token ? ('已登录，Token 长度 ' + token.length) : '未登录';
  }

  function headers(json) {
    var h = {};
    if (token) { h['Authorization'] = 'Bearer ' + token; }
    if (json) { h['Content-Type'] = 'application/json'; }
    return h;
  }

  async function api(method, path, payload) {
    var body = payload == null ? null : JSON.stringify(payload);
    var start = Date.now();
    var res = await fetch(origin + path, { method: method, headers: headers(body != null), body: body });
    var text = await res.text();
    showResp(method, path, res.status, text, Date.now() - start);
    var data = null;
    try { data = JSON.parse(text); } catch (e) {}
    return { ok: res.ok, status: res.status, data: data };
  }

  function readForm() {
    var styles = document.getElementById('wizStyles').value.split(/[,，]/).map(function (s) { return s.trim(); }).filter(Boolean).slice(0, 3);
    return {
      styles: styles,
      duration_seconds: parseInt(document.getElementById('wizDuration').value, 10),
      quality: document.getElementById('wizQuality').value,
      stems: document.getElementById('wizStems').checked,
      prompt: document.getElementById('wizPrompt').value.trim(),
      title: document.getElementById('wizTitle').value.trim(),
      vocal: document.getElementById('wizVocal').value,
      auto_publish: document.getElementById('wizAutoPublish').checked
    };
  }

  function needLogin() {
    if (token) { return false; }
    wizMeta.innerHTML = '<span style="color:var(--bad)">请先在上方登录，登录后会自动携带 Token</span>';
    return true;
  }

  function sleep(ms) { return new Promise(function (r) { setTimeout(r, ms); }); }

  async function doQuote(form) {
    setStep('quote', 'run', '请求中…');
    var r = await api('POST', '/api/v1/credit/quotes', {
      styles: form.styles, duration_seconds: form.duration_seconds, quality: form.quality, stems: form.stems
    });
    if (!r.ok || !r.data || !r.data.data) {
      setStep('quote', 'fail', '失败 HTTP ' + r.status);
      return null;
    }
    wiz.quoteId = r.data.data.quote_id;
    wiz.total = r.data.data.total;
    setStep('quote', 'done', 'quote_id=' + wiz.quoteId + '　' + wiz.total + ' 积分');
    return r.data.data;
  }

  async function doGenerate(form) {
    if (!wiz.quoteId) {
      setStep('generate', 'fail', '缺少 quote_id，请先报价');
      return null;
    }
    if (!form.prompt) {
      setStep('generate', 'fail', 'prompt 必填');
      return null;
    }
    setStep('generate', 'run', '请求中…');
    var r = await api('POST', '/api/v1/generations', {
      quote_id: wiz.quoteId,
      prompt: form.prompt,
      styles: form.styles,
      duration_seconds: form.duration_seconds,
      quality: form.quality,
      stems: form.stems,
      title: form.title,
      vocal: form.vocal,
      visibility: 'public',
      auto_publish: form.auto_publish
    });
    if (!r.ok || !r.data || !r.data.data) {
      var msg = r.data && r.data.message ? r.data.message : ('失败 HTTP ' + r.status);
      setStep('generate', 'fail', msg);
      return null;
    }
    wiz.genId = r.data.data.id;
    setStep('generate', 'done', 'generation_id=' + wiz.genId);
    return r.data.data;
  }

  async function doWatch() {
    if (!wiz.genId || wiz.watching) { return null; }
    wiz.watching = true;
    setStep('progress', 'run', '排队中…');
    var finalGen = null;
    for (var i = 0; i < 120; i++) {
      await sleep(1000);
      var r = await api('GET', '/api/v1/generations/' + wiz.genId);
      if (!r.ok || !r.data || !r.data.data) {
        setStep('progress', 'fail', '查询失败 HTTP ' + r.status);
        wiz.watching = false;
        return null;
      }
      var gen = r.data.data;
      bar.style.width = Math.max(0, Math.min(100, gen.progress || 0)) + '%';
      setStep('progress', 'run', (gen.stage || gen.status) + '　' + (gen.progress || 0) + '%');
      if (gen.status === 'succeeded' || gen.status === 'failed' || gen.status === 'cancelled') {
        finalGen = gen;
        break;
      }
    }
    wiz.watching = false;
    if (!finalGen) {
      setStep('progress', 'fail', '超时未完成');
      return null;
    }
    if (finalGen.status !== 'succeeded') {
      setStep('progress', 'fail', finalGen.status + (finalGen.error ? '：' + finalGen.error : ''));
      return finalGen;
    }
    bar.style.width = '100%';
    wiz.workId = finalGen.work_id || '';
    setStep('progress', 'done', '生成完成，work_id=' + (wiz.workId || '无'));

    if (wiz.workId) {
      var wr = await api('GET', '/api/v1/works/' + wiz.workId);
      if (wr.ok && wr.data && wr.data.data && wr.data.data.audio_url) {
        player.src = origin + wr.data.data.audio_url;
        player.style.display = 'block';
      }
      if (finalGen.auto_publish) {
        setStep('publish', 'done', '已自动发布');
      } else {
        setStep('publish', 'run', '待发布，点击「③ 发布作品」');
      }
    }
    return finalGen;
  }

  async function doPublish(form) {
    if (!wiz.workId) {
      setStep('publish', 'fail', '缺少 work_id，请先生成作品');
      return;
    }
    setStep('publish', 'run', '请求中…');
    var r = await api('POST', '/api/v1/works/' + wiz.workId + '/publish', {
      title: form.title, visibility: 'public', tags: form.styles
    });
    if (!r.ok) {
      setStep('publish', 'fail', '失败 HTTP ' + r.status);
      return;
    }
    setStep('publish', 'done', '已发布：' + wiz.workId);
    wizMeta.textContent = '全链路完成，作品已发布';
  }

  document.getElementById('loginBtn').addEventListener('click', async function () {
    var start = Date.now();
    var res = await fetch(origin + '/api/v1/auth/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        phone: document.getElementById('phone').value.trim(),
        code: document.getElementById('code').value.trim()
      })
    });
    var text = await res.text();
    showResp('POST', '/api/v1/auth/login', res.status, text, Date.now() - start);
    var data = null;
    try { data = JSON.parse(text); } catch (e) {}
    if (res.ok && data && data.data && data.data.access_token) {
      token = data.data.access_token;
      localStorage.setItem(TOKEN_KEY, token);
      refreshAuthStatus();
    }
  });

  var presets = [
    { name: '健康检查', method: 'GET', path: '/health', body: '' },
    { name: '推荐流', method: 'GET', path: '/api/v1/feed/recommend', body: '' },
    { name: '广场最新', method: 'GET', path: '/api/v1/plaza?sort=latest', body: '' },
    { name: '我的信息', method: 'GET', path: '/api/v1/me', body: '' },
    { name: '我的积分', method: 'GET', path: '/api/v1/me/credits', body: '' },
    { name: '签到状态', method: 'GET', path: '/api/v1/checkins/status', body: '' },
    { name: '每日签到', method: 'POST', path: '/api/v1/checkins', body: '' },
    { name: '作品列表', method: 'GET', path: '/api/v1/works?page_size=5', body: '' },
    { name: '积分套餐', method: 'GET', path: '/api/v1/credit/packages', body: '' },
    { name: '我的作品', method: 'GET', path: '/api/v1/me/works', body: '' },
    { name: '通知', method: 'GET', path: '/api/v1/notifications', body: '' },
    { name: '会话', method: 'GET', path: '/api/v1/conversations', body: '' }
  ];

  var presetBox = document.getElementById('presets');
  presets.forEach(function (p) {
    var btn = document.createElement('button');
    btn.textContent = p.name;
    btn.addEventListener('click', function () {
      document.getElementById('method').value = p.method;
      document.getElementById('path').value = p.path;
      document.getElementById('body').value = p.body;
      api(p.method, p.path, p.body ? JSON.parse(p.body) : null);
    });
    presetBox.appendChild(btn);
  });

  document.getElementById('sendBtn').addEventListener('click', async function () {
    var method = document.getElementById('method').value;
    var path = document.getElementById('path').value.trim();
    var raw = document.getElementById('body').value.trim();
    var payload = null;
    if (raw) {
      try { payload = JSON.parse(raw); } catch (e) {
        wizMeta.textContent = '请求体不是合法 JSON';
        return;
      }
    }
    await api(method, path, payload);
  });
  document.getElementById('clearBtn').addEventListener('click', function () {
    output.textContent = '';
    respMeta.textContent = '等待发送…';
  });

  document.getElementById('btnQuote').addEventListener('click', async function () {
    if (needLogin()) { return; }
    resetSteps();
    await doQuote(readForm());
  });

  document.getElementById('btnGen').addEventListener('click', async function () {
    if (needLogin()) { return; }
    var form = readForm();
    if (!wiz.quoteId) {
      if (!(await doQuote(form))) { return; }
    }
    if (!(await doGenerate(form))) { return; }
    await doWatch();
  });

  document.getElementById('btnPublish').addEventListener('click', async function () {
    if (needLogin()) { return; }
    await doPublish(readForm());
  });

  document.getElementById('runAll').addEventListener('click', async function () {
    if (needLogin()) { return; }
    resetSteps();
    var form = readForm();
    wizMeta.textContent = '执行中：报价 → 生成 → 轮询进度 → 发布';
    if (!(await doQuote(form))) { wizMeta.textContent = '报价失败，链路终止'; return; }
    if (!(await doGenerate(form))) { wizMeta.textContent = '创建任务失败，链路终止'; return; }
    var gen = await doWatch();
    if (!gen || gen.status !== 'succeeded') { wizMeta.textContent = '生成未成功，链路终止'; return; }
    if (gen.auto_publish) { wizMeta.textContent = '链路完成，作品已自动发布'; return; }
    await doPublish(form);
  });

  refreshAuthStatus();
})();
</script>
</body>
</html>
`
