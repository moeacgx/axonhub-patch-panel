package ui

import (
	"encoding/json"
	"html/template"
	"net/http"
)

type Config struct {
	UpstreamURL           string `json:"upstreamUrl"`
	RedisAddr             string `json:"redisAddr"`
	KeyPrefix             string `json:"keyPrefix"`
	TTL                   string `json:"ttl"`
	RespectExistingThread bool   `json:"respectExistingThread"`
	RespectExistingTrace  bool   `json:"respectExistingTrace"`
}

func Handler(cfg Config) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = panelTemplate.Execute(w, cfg)
	})
	mux.HandleFunc("/config.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(cfg)
	})
	return mux
}

var panelTemplate = template.Must(template.New("panel").Parse(`<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>AxonHub Patch Panel</title>
  <style>
    :root {
      color-scheme: light;
      --bg: #f6f7f4;
      --ink: #1f2a24;
      --muted: #66736b;
      --line: #d9ded6;
      --accent: #2f6f5f;
      --panel: #ffffff;
      --warn: #9a5b21;
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      font-family: ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
      background: linear-gradient(180deg, #f7f8f4 0%, #eef2ed 100%);
      color: var(--ink);
    }
    main {
      width: min(980px, calc(100vw - 32px));
      margin: 48px auto;
    }
    header {
      display: flex;
      justify-content: space-between;
      gap: 24px;
      align-items: flex-start;
      margin-bottom: 28px;
    }
    h1 {
      margin: 0 0 8px;
      font-size: 32px;
      font-weight: 760;
      letter-spacing: 0;
    }
    p {
      margin: 0;
      color: var(--muted);
      line-height: 1.6;
    }
    .status {
      display: inline-flex;
      align-items: center;
      gap: 8px;
      border: 1px solid var(--line);
      background: var(--panel);
      padding: 8px 12px;
      border-radius: 8px;
      font-size: 14px;
      white-space: nowrap;
    }
    .dot {
      width: 9px;
      height: 9px;
      border-radius: 999px;
      background: #2f9b6d;
      box-shadow: 0 0 0 3px rgba(47,155,109,.15);
    }
    .grid {
      display: grid;
      grid-template-columns: repeat(2, minmax(0, 1fr));
      gap: 16px;
    }
    section {
      background: var(--panel);
      border: 1px solid var(--line);
      border-radius: 8px;
      padding: 20px;
    }
    h2 {
      margin: 0 0 16px;
      font-size: 16px;
    }
    dl {
      display: grid;
      gap: 12px;
      margin: 0;
    }
    .row {
      display: grid;
      grid-template-columns: 170px 1fr;
      gap: 12px;
      align-items: start;
    }
    dt {
      color: var(--muted);
      font-size: 13px;
    }
    dd {
      margin: 0;
      font-family: ui-monospace, SFMono-Regular, Consolas, "Liberation Mono", monospace;
      overflow-wrap: anywhere;
      font-size: 13px;
    }
    .note {
      border-color: rgba(154,91,33,.25);
      background: #fffaf1;
      color: var(--warn);
    }
    @media (max-width: 720px) {
      header, .grid { grid-template-columns: 1fr; display: grid; }
      .row { grid-template-columns: 1fr; gap: 4px; }
      main { margin-top: 28px; }
    }
  </style>
</head>
<body>
  <main>
    <header>
      <div>
        <h1>AxonHub Patch Panel</h1>
        <p>补丁代理正在为请求补齐 AH-Thread-Id 与 AH-Trace-Id，线程与追踪展示继续使用 AxonHub。</p>
      </div>
      <div class="status"><span class="dot"></span>运行中</div>
    </header>

    <div class="grid">
      <section>
        <h2>代理配置</h2>
        <dl>
          <div class="row"><dt>上游 AxonHub</dt><dd>{{.UpstreamURL}}</dd></div>
          <div class="row"><dt>Redis</dt><dd>{{.RedisAddr}}</dd></div>
          <div class="row"><dt>Key 前缀</dt><dd>{{.KeyPrefix}}</dd></div>
          <div class="row"><dt>映射 TTL</dt><dd>{{.TTL}}</dd></div>
        </dl>
      </section>

      <section>
        <h2>Header 策略</h2>
        <dl>
          <div class="row"><dt>保留已有 Thread</dt><dd>{{.RespectExistingThread}}</dd></div>
          <div class="row"><dt>保留已有 Trace</dt><dd>{{.RespectExistingTrace}}</dd></div>
          <div class="row"><dt>Thread Header</dt><dd>AH-Thread-Id</dd></div>
          <div class="row"><dt>Trace Header</dt><dd>AH-Trace-Id</dd></div>
        </dl>
      </section>

      <section class="note">
        <h2>使用方式</h2>
        <p>把客户端 base_url 指向本服务，API Key 继续使用 AxonHub 的 Key。请求会被转发到上游 AxonHub，并自动补齐追踪头。</p>
      </section>
    </div>
  </main>
</body>
</html>`))
