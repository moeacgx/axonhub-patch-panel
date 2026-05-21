# AxonHub Patch Panel

独立部署在 AxonHub 前面的轻量补丁代理。

当前补丁会为 OpenAI Chat Completions、OpenAI Responses、Anthropic Messages 请求自动补齐：

- `AH-Thread-Id`：通过 Redis 维护会话状态映射，尽量让同一会话稳定落到同一线程。
- `AH-Trace-Id`：每次请求新生成 `at-<uuid>`。

可选补丁还可以把 Claude Opus 4.7 的旧 thinking 请求改写为 adaptive thinking，用于兼容不再接受 `thinking.type=enabled` 的上游。

AxonHub 主程序不需要修改，线程页和追踪页继续使用 AxonHub 自带页面。
面板支持密码保护和在线保存配置，保存后会写入 `/data/config.json` 并立即影响新请求。

## Docker Compose

### 推荐：加入 AxonHub 同一个 Docker 网络

如果你的 AxonHub 容器名是 `axonhub-app`，容器内端口是 `8090`，优先使用这个方式。

先查看 AxonHub 所在网络：

```bash
docker inspect axonhub-app --format '{{json .NetworkSettings.Networks}}'
```

把 `docker-compose.example.yml` 里的网络名改成查到的名称，例如：

```yaml
networks:
  axonhub-net:
    external: true
    name: axonhub_default
```

然后启动：

```bash
docker compose -f docker-compose.example.yml up -d --build
```

### 备选：通过宿主机端口转发

如果你暂时不想处理 Docker 网络，也可以直接走截图里的宿主机端口 `38098`：

```bash
docker compose -f docker-compose.host-port.example.yml up -d --build
```

这个方式会把请求转发到：

```text
http://host.docker.internal:38098
```

### 客户端改法

把客户端的 `base_url` 指向补丁面板，例如：

```text
http://your-host:8080/v1
```

API Key 继续填写 AxonHub 的 Key。

## 环境变量

- `AXONHUB_URL`：必填，上游 AxonHub 地址。
- `REDIS_ADDR`：默认 `redis:6379`。
- `REDIS_PASSWORD`：Redis 密码，可选。
- `REDIS_DB`：Redis DB，默认 `0`。
- `SETTINGS_PATH`：面板保存配置路径，默认 `/data/config.json`。
- `PANEL_USERNAME`：面板用户名，默认 `admin`。
- `PANEL_PASSWORD`：面板密码。为空时不启用密码保护，生产环境务必设置。
- `THREAD_ENABLED`：是否启用 `AH-Thread-Id` 注入，默认 `true`。
- `TRACE_ENABLED`：是否启用 `AH-Trace-Id` 注入，默认 `true`。
- `THREAD_TTL`：线程映射保存时间，默认 `720h`。
- `KEY_PREFIX`：Redis key 前缀，默认 `ahpatch`。
- `RESPECT_EXISTING_THREAD`：是否保留客户端已有 `AH-Thread-Id`，默认 `true`。
- `RESPECT_EXISTING_TRACE`：是否保留客户端已有 `AH-Trace-Id`，默认 `false`。
- `CLAUDE_THINKING_REWRITE_ENABLED`：是否启用 Claude thinking 兼容补丁，默认 `false`。
- `CLAUDE_THINKING_REWRITE_MODELS`：命中的模型 ID，逗号分隔，默认 `claude-opus-4-7`。
- `CLAUDE_THINKING_REWRITE_EFFORT`：写入 `output_config.effort` 的值，默认 `xhigh`。

## 工作方式

每次请求都会生成新的 `AH-Trace-Id`。

`AH-Thread-Id` 会按下面优先级决定：

1. 如果客户端已有 `AH-Thread-Id`，且 `RESPECT_EXISTING_THREAD=true`，直接沿用。
2. 如果 OpenAI Responses 带了 `previous_response_id`，优先用它查 Redis 映射。
3. 如果请求里有 `user` / Anthropic `metadata.user_id`，用它作为会话线索。
4. 否则提取并规范化消息，去掉最后一个 user turn 后计算上下文 hash。
5. Redis 找不到映射时创建新的线程 ID，并在响应结束后记住新状态。

普通 JSON 响应和 SSE 流式响应都会更新线程状态。

## Claude Thinking 兼容补丁

启用后，补丁只处理 `/v1/messages` 且模型命中 `CLAUDE_THINKING_REWRITE_MODELS` 的请求。

当请求体里存在旧格式：

```json
{
  "thinking": {
    "type": "enabled",
    "budget_tokens": 1024
  }
}
```

会在转发给 AxonHub 前改写为：

```json
{
  "thinking": {
    "type": "adaptive"
  },
  "output_config": {
    "effort": "xhigh"
  }
}
```

要启用这个补丁，把 compose 里的环境变量改成：

```yaml
CLAUDE_THINKING_REWRITE_ENABLED: "true"
CLAUDE_THINKING_REWRITE_MODELS: claude-opus-4-7
CLAUDE_THINKING_REWRITE_EFFORT: xhigh
```

也可以在 `/_panel/` 页面里直接启用、修改 effort、添加命中模型。这里添加模型只影响补丁规则，不会修改 AxonHub 主程序里的模型表。

面板页面在：

```text
/_panel/
```

如果设置了 `PANEL_PASSWORD`，浏览器访问面板时使用 Basic Auth 登录。
