# NotifyRelay

[English](README.md) · [完整配置](docs/configuration.md) · [架构](docs/architecture.md)

```sh
git clone https://github.com/Syt100/notify-relay.git
cd notify-relay
```

把自建 ntfy 通知转发到 WxPusher 的轻量 Go 服务。业务系统继续只向 ntfy
发送消息，WxPusher 凭证仅保存在 bridge。单个静态程序，内置 bbolt 持久队列，
不依赖 Python，也不需要单独运行数据库。

## 功能

- ntfy Token 鉴权、多 topic 独立订阅、自动重连、断线缓存补读。
- 消息落盘和订阅游标在同一个事务内提交，按 topic + 消息 ID 去重。
- 磁盘队列、指数退避和随机抖动、所有投递合计最多每秒一次。
- 优先级过滤、标题 Emoji、标签、附件链接、view action 链接、点击跳转。
- Markdown 格式转发、主题来源标识、失败消息隔离与手动重新投递。
- JSON 日志、健康/就绪检查、凭证文件、非 root 容器。
- Linux amd64 / arm64 / armv7 发布流程，Compose 示例限制内存为 64 MiB。

这是初版代码；[验证记录](docs/verification.md)列出实测数据和未验证的部分。
`GOMEMLIMIT=32MiB` 是 Go 运行时软目标，不等于进程或容器总内存上限。

## 使用 GHCR 预构建镜像

小内存服务器可以直接拉取镜像，无需安装 Go 或本地编译。先按下文配置
`.env` 和 ntfy 所在的 Docker 网络，然后执行：

```sh
docker compose -f compose.ghcr.yaml pull
docker compose -f compose.ghcr.yaml up -d
docker compose -f compose.ghcr.yaml exec wxpusher-bridge /bridge readycheck
```

默认镜像为 `ghcr.io/syt100/notify-relay:main`，每次 `main` 的全部 CI 检查
通过后自动构建并发布，支持 amd64、arm64 和 arm/v7。需要固定版本时，
在 `.env` 中将 `NOTIFY_RELAY_IMAGE` 设为 `:sha-<完整提交 SHA>` 标签或镜像
digest。GHCR 软件包设为 Public 后才能匿名拉取，步骤见
[发布指南](docs/releasing.md)。正式版本标签和 `latest` 由版本发布流程生成。

## 对接已有 ntfy

将项目放在服务器上，执行：

```sh
cp .env.example .env
chmod 600 .env
```

编辑 `.env`，填入专门给 bridge 使用的 ntfy 只读 Token，以及 WxPusher SPT。
如果订阅多个主题，写成 `NTFY_TOPICS=notify,backup,security`。
这个账号必须具备每个主题的读取权限。

在 `.env` 中设置 `NTFY_DOCKER_NETWORK`，指定已有 ntfy 容器所在的 Docker
网络，默认值为 `ntfy`。程序内部直接连接 `http://ntfy`，无需经过反向代理。
ntfy 容器需要在同一网络，并能通过 `ntfy` 这个名字访问；其他地址可修改
`NTFY_BASE_URL`。例如使用 Nginx Proxy Manager 时，可填写对应的共享网络名。

```sh
docker compose up -d --build
docker compose logs -f wxpusher-bridge
docker compose exec wxpusher-bridge /bridge healthcheck
docker compose exec wxpusher-bridge /bridge readycheck
```

无需暴露端口；`/healthz` 表示进程存活，`/readyz` 还检查订阅连接与最近一次
投递结果。Docker 的健康检查使用前者，因此 WxPusher 临时故障不会让进程
被误判为死亡。Docker Compose 本身也不会因为 unhealthy 自动重启。

默认通过命名卷持久保存状态。如果更喜欢绑定挂载：

```sh
mkdir -p data
sudo chown 1000:1001 data
chmod 700 data
```

把 `bridge-data:/data` 改成 `./data:/data`。容器 UID/GID 为 `1000:1001`。
镜像没有 shell、wget、curl，检查接口请使用上面的程序子命令。

也可以把 service 合并到 ntfy 原来的 Compose 中。此时 `build.context` 改成
项目目录，例如 `./notify-relay`，并保留相应的 `volumes` 声明。

## 小内存服务器

Go 编译器只在构建阶段使用。运行时只有一个程序和 CA 证书；队列按需读盘，
不会把全部消息正文一次性加载进 Go 堆。

示例设置 `GOMEMLIMIT=32MiB`、`GOGC=50`、`GOMAXPROCS=1` 和容器硬限制
`mem_limit: 64m`。数据库内存映射、网络缓冲和容器文件页缓存仍占用内存，
高流量/大队列时要看 `docker stats`，不能把软目标当成保证。

服务器不方便编译时，可在其他机器运行 `make build`，复制生成的程序；发布
项目并推送版本 tag 后，GitHub Actions 会生成二进制压缩包和多架构镜像。
也可以使用上文的 GHCR 预构建镜像，无需在服务器上编译。

## 消息与可靠性

有标题时，摘要显示 `[主题] 标题`，正文也保留来源。ntfy 标记为
`content_type: text/markdown` 的消息使用 WxPusher Markdown 模式；发送端
需启用 `Markdown: yes`，或在 ntfy JSON 发布请求中设置 `markdown: true`。
仅包含 Markdown 语法的普通消息仍按纯文本发送。若 WxPusher 用业务码
1001 拒绝 Markdown，会尝试一次带说明的纯文本降级，保留原始语法。

正文按字符数、UTF-8 字节、HTML 转义与纯文本换行膨胀限制，超长时加省略号。
Markdown 最终渲染大小由 WxPusher 决定，不能仅靠本地检查保证接收。
`click` 最多 1000 字符；更长时放入受长度限制的正文，而不单独截断链接。

优先级 1～5 映射为 💤、ℹ️、无前缀、⚠️、🚨，默认全部转发。
`FORWARD_MIN_PRIORITY=4` 可以仅发送 high/urgent 消息。这不会改变手机系统
通知等级，也不能绕过勿扰模式。`click` 保留为跳转 URL；附件只传链接，
HTTP/broadcast/copy action 不会执行。图片预览、自定义图标、通知更新、
清除与删除尚未同步。

首次启动只接收新消息，之后按保存的时间补读并去重。网络故障、限流和
HTTP 认证错误继续退避重试；HTTP 400/413/422 立即隔离。通用业务码
1001 不能直接认定为永久错误，连续五次投递失败后才隔离，停止自动发送。
处理过的 ID 默认保留七天。健康接口中的 `processed_retained` 同时包括
API 已接收和被优先级过滤的消息。

**需要知道的边界：**

- ntfy 缓存过期或关闭后，bridge 无法补回尚未落到本地队列的消息。
- 队列满时暂停订阅，不推进游标。超长故障仍可能超过 ntfy 缓存时间。
- ntfy 若截断补读结果，会继续处理返回的消息，但记录错误并让 `/readyz`
  保持异常，显示 `replay_truncated: true`。检查漏失范围后，重启可确认并清除
  这个运行期标记；已经被服务端省略的旧消息不会自动恢复。
- WxPusher 接收成功但响应丢失，或者进程在本地确认前崩溃，重试可能产生
  重复通知。这里提供的是“至少一次”，不是端到端“恰好一次”。
- API 返回成功代表已创建推送任务，不代表 Redmi 已展示系统通知。
- 隔离消息保留原文、去重信息和失败原因，与待发消息一起计入
  `MAX_PENDING`。有隔离消息时 `/readyz` 保持异常，`/healthz` 仍检查存活。
  修正配置后可停止服务，用 `failed list` 查看元数据，用
  `failed retry 主题 消息ID` 重新投递；[操作示例](docs/configuration.md#failed-queue-maintenance)。
- 同一个状态文件只允许一个实例使用。请使用本地磁盘，定期备份状态目录；
  清理记录后数据库文件不一定立即缩小。
- 状态文件采用 bbolt 格式，不兼容 SQLite，也不能复用其他应用的数据库。
- 本次升级自动将状态格式从 1 升为 2；升级前请停机备份状态文件。旧版
  程序拒绝打开格式 2，回退时需要使用升级前备份。

完整参数、CI、镜像发布与贡献说明见 [English README](README.md)、
[配置文档](docs/configuration.md)、[发布指南](docs/releasing.md) 与
[贡献指南](CONTRIBUTING.md)。本项目采用 [MIT 许可证](LICENSE)，不隶属于
ntfy 或 WxPusher 官方。
