package ui

import (
	"crypto/subtle"
	"encoding/json"
	"html/template"
	"net/http"

	"axonhub-patch-panel/internal/settings"
)

type Config struct {
	UpstreamURL string            `json:"upstreamUrl"`
	RedisAddr   string            `json:"redisAddr"`
	Settings    settings.Settings `json:"settings"`
}

type Options struct {
	Config       func() Config
	Update       func(settings.Settings) error
	Username     string
	Password     string
	AuthRequired bool
}

func Handler(opts Options) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if !checkAuth(w, r, opts) {
			return
		}
		if r.Method == http.MethodPost {
			handleFormUpdate(w, r, opts)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = panelTemplate.Execute(w, panelView{
			Config:     opts.Config(),
			AuthStatus: authStatus(opts),
		})
	})
	mux.HandleFunc("/config.json", func(w http.ResponseWriter, r *http.Request) {
		if !checkAuth(w, r, opts) {
			return
		}
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(opts.Config())
		case http.MethodPost:
			var next settings.Settings
			if err := json.NewDecoder(r.Body).Decode(&next); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if err := opts.Update(next); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(opts.Config())
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	return mux
}

func handleFormUpdate(w http.ResponseWriter, r *http.Request, opts Options) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	current := opts.Config().Settings
	next := settings.Settings{
		ThreadEnabled:                formBool(r, "threadEnabled"),
		TraceEnabled:                 formBool(r, "traceEnabled"),
		KeyPrefix:                    r.FormValue("keyPrefix"),
		ThreadTTL:                    r.FormValue("threadTtl"),
		RespectExistingThread:        formBool(r, "respectExistingThread"),
		RespectExistingTrace:         formBool(r, "respectExistingTrace"),
		ClaudeThinkingRewriteEnabled: formBool(r, "claudeThinkingRewriteEnabled"),
		ClaudeThinkingRewriteModels:  settings.ModelsFromText(r.FormValue("claudeThinkingRewriteModels")),
		ClaudeThinkingRewriteEffort:  r.FormValue("claudeThinkingRewriteEffort"),
	}
	if next.KeyPrefix == "" {
		next.KeyPrefix = current.KeyPrefix
	}
	if next.ThreadTTL == "" {
		next.ThreadTTL = current.ThreadTTL
	}
	if err := opts.Update(next); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/_panel/", http.StatusSeeOther)
}

func formBool(r *http.Request, key string) bool {
	return r.FormValue(key) == "on" || r.FormValue(key) == "true"
}

func checkAuth(w http.ResponseWriter, r *http.Request, opts Options) bool {
	if !opts.AuthRequired {
		return true
	}
	username, password, ok := r.BasicAuth()
	if !ok || !safeEqual(username, opts.Username) || !safeEqual(password, opts.Password) {
		w.Header().Set("WWW-Authenticate", `Basic realm="AxonHub Patch Panel"`)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return false
	}
	return true
}

func safeEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func authStatus(opts Options) string {
	if opts.AuthRequired {
		return "已启用"
	}
	return "未启用"
}

type panelView struct {
	Config     Config
	AuthStatus string
}

func (v panelView) ModelsText() string {
	return settings.ModelsText(v.Config.Settings.ClaudeThinkingRewriteModels)
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
      --bg: #f4f1ea;
      --ink: #1e251f;
      --muted: #6b746d;
      --line: #ded7c9;
      --accent: #b95f2a;
      --accent-ink: #fff9f2;
      --panel: #fffdf8;
      --note: #fff3dc;
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      font-family: "Segoe UI", "Noto Sans SC", sans-serif;
      background:
        radial-gradient(circle at 10% 0%, rgba(185,95,42,.14), transparent 32%),
        linear-gradient(180deg, #faf7ef 0%, #eee8db 100%);
      color: var(--ink);
    }
    main { width: min(1060px, calc(100vw - 32px)); margin: 42px auto; }
    header {
      display: flex;
      justify-content: space-between;
      gap: 24px;
      align-items: flex-start;
      margin-bottom: 24px;
    }
    h1 { margin: 0 0 8px; font-size: 34px; letter-spacing: -.02em; }
    p { margin: 0; color: var(--muted); line-height: 1.65; }
    .status {
      display: inline-flex;
      align-items: center;
      gap: 8px;
      border: 1px solid var(--line);
      background: var(--panel);
      padding: 8px 12px;
      border-radius: 999px;
      font-size: 14px;
      white-space: nowrap;
    }
    .dot {
      width: 9px;
      height: 9px;
      border-radius: 999px;
      background: #2f9b6d;
      box-shadow: 0 0 0 3px rgba(47,155,109,.16);
    }
    .grid {
      display: grid;
      grid-template-columns: 1fr 1fr;
      gap: 16px;
    }
    section {
      background: rgba(255,253,248,.92);
      border: 1px solid var(--line);
      border-radius: 18px;
      padding: 20px;
      box-shadow: 0 16px 46px rgba(83,65,37,.08);
    }
    section.wide { grid-column: 1 / -1; }
    h2 { margin: 0 0 16px; font-size: 17px; }
    dl, .form-grid { display: grid; gap: 12px; margin: 0; }
    .row {
      display: grid;
      grid-template-columns: 180px 1fr;
      gap: 12px;
      align-items: start;
    }
    dt, label, .hint { color: var(--muted); font-size: 13px; }
    dd {
      margin: 0;
      font-family: Consolas, "Liberation Mono", monospace;
      overflow-wrap: anywhere;
      font-size: 13px;
    }
    input[type="text"], textarea {
      width: 100%;
      border: 1px solid var(--line);
      border-radius: 12px;
      background: #fffaf2;
      color: var(--ink);
      padding: 10px 12px;
      font: 14px Consolas, "Liberation Mono", monospace;
      outline: none;
    }
    textarea { min-height: 98px; resize: vertical; }
    input[type="checkbox"] { transform: translateY(2px); }
    .toggle-row {
      display: flex;
      align-items: center;
      gap: 10px;
      min-height: 38px;
    }
    .actions { display: flex; gap: 10px; margin-top: 18px; }
    button {
      border: 0;
      border-radius: 999px;
      background: var(--accent);
      color: var(--accent-ink);
      padding: 10px 18px;
      font-weight: 700;
      cursor: pointer;
    }
    .note {
      border-color: rgba(185,95,42,.24);
      background: var(--note);
    }
    @media (max-width: 760px) {
      header, .grid { display: grid; grid-template-columns: 1fr; }
      .row { grid-template-columns: 1fr; gap: 5px; }
      main { margin-top: 28px; }
    }
  </style>
</head>
<body>
  <main>
    <header>
      <div>
        <h1>AxonHub Patch Panel</h1>
        <p>补丁控制台：管理 trace/thread 注入、Claude thinking 兼容补丁和运行状态。</p>
      </div>
      <div class="status"><span class="dot"></span>运行中 · 密码{{.AuthStatus}}</div>
    </header>

    <div class="grid">
      <section>
        <h2>代理状态</h2>
        <dl>
          <div class="row"><dt>上游 AxonHub</dt><dd>{{.Config.UpstreamURL}}</dd></div>
          <div class="row"><dt>Redis</dt><dd>{{.Config.RedisAddr}}</dd></div>
          <div class="row"><dt>配置来源</dt><dd>/data/config.json</dd></div>
        </dl>
      </section>

      <section class="note">
        <h2>说明</h2>
        <p>保存后会立即影响新请求，不需要重启容器。这里添加 Claude 模型是添加到补丁命中列表，不会修改 AxonHub 主程序模型表。</p>
      </section>

      <section class="wide">
        <h2>补丁设置</h2>
        <form method="post" action="/_panel/">
          <div class="form-grid">
            <div class="row">
              <label>Thread 注入</label>
              <div class="toggle-row"><input type="checkbox" name="threadEnabled" {{if .Config.Settings.ThreadEnabled}}checked{{end}}> 启用 AH-Thread-Id</div>
            </div>
            <div class="row">
              <label>Trace 注入</label>
              <div class="toggle-row"><input type="checkbox" name="traceEnabled" {{if .Config.Settings.TraceEnabled}}checked{{end}}> 启用 AH-Trace-Id</div>
            </div>
            <div class="row">
              <label>保留已有 Thread</label>
              <div class="toggle-row"><input type="checkbox" name="respectExistingThread" {{if .Config.Settings.RespectExistingThread}}checked{{end}}> 客户端已带时不覆盖</div>
            </div>
            <div class="row">
              <label>保留已有 Trace</label>
              <div class="toggle-row"><input type="checkbox" name="respectExistingTrace" {{if .Config.Settings.RespectExistingTrace}}checked{{end}}> 客户端已带时不覆盖</div>
            </div>
            <div class="row">
              <label for="keyPrefix">Redis Key 前缀</label>
              <input id="keyPrefix" name="keyPrefix" type="text" value="{{.Config.Settings.KeyPrefix}}">
            </div>
            <div class="row">
              <label for="threadTtl">Thread TTL</label>
              <input id="threadTtl" name="threadTtl" type="text" value="{{.Config.Settings.ThreadTTL}}">
            </div>
            <div class="row">
              <label>Claude Thinking 重写</label>
              <div class="toggle-row"><input type="checkbox" name="claudeThinkingRewriteEnabled" {{if .Config.Settings.ClaudeThinkingRewriteEnabled}}checked{{end}}> 启用 enabled → adaptive</div>
            </div>
            <div class="row">
              <label for="claudeThinkingRewriteModels">命中模型</label>
              <textarea id="claudeThinkingRewriteModels" name="claudeThinkingRewriteModels">{{.ModelsText}}</textarea>
            </div>
            <div class="row">
              <label for="claudeThinkingRewriteEffort">Effort</label>
              <input id="claudeThinkingRewriteEffort" name="claudeThinkingRewriteEffort" type="text" value="{{.Config.Settings.ClaudeThinkingRewriteEffort}}">
            </div>
          </div>
          <div class="actions">
            <button type="submit">保存设置</button>
          </div>
        </form>
      </section>
    </div>
  </main>
</body>
</html>`))
