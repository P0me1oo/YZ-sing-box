# YZboard-Node 兼容补丁

当前发布 [v1.16.0](https://github.com/P0me1oo/YZ-sing-box/releases/tag/v1.16.0) 固定源码为 `ad0c0cfbc7c3827f3058aa666d9f1a3b0e0fd085`，安装包及构建来源见[发布核验](release-1.16.0.md)。

## v1.16.0 DHT 识别与逐报文拦截（2026-11-16）

产品版本 `v1.16.0`，改动基于 `3c602031097cc8e5a690c3b57c7a37ee67caf6c7`。Agent v2.38.0 通过已发布固定版本消费本补丁；不把本地路径写入正式依赖。版本记录见 [CHANGELOG.md](../CHANGELOG.md)。

`common/dht` 严格验证完整、规范编码的 KRPC 字典、事务编号、消息类型和方法字段。覆盖 ping、find_node、get_peers、announce_peer，以及 IPv6、scrape、只读节点、get/put、可变数据和 sample_infohashes 扩展。拒绝截断、拼接或尾随数据、重复及无序字典键、无效长度/整数、字段缺失或类型不符、未知查询和非标准错误码。原地扫描并只保存必要字段，解析不构建字典和列表；限制为 65,507 字节、16 层嵌套及 4,096 个值。按 BEP 32 忽略未知 `want` 字符串，BEP 44 存储值允许长字典键及规范编码的大整数；不验证节点身份或可变数据签名的密码学有效性。

识别结果归入既有 `bittorrent`，不新增面板协议。只有显式 BT 条件指向 block 出站或 reject 动作时才包装 UDP 读取路径。有效 DHT 按每份报文的目的地址重新执行完整路由匹配，保留入站、用户、地址、端口、网络、规则集及规则先后顺序；更早命中的直连或中转规则仍优先。仅命中 BT 拦截规则的报文被丢弃，不把整个会话永久标为 BT，也不关闭同会话的普通 UDP。

路由快照与逐报文匹配使用同一读锁。已带检查器的会话在规则热更新后使用新条件；没有 BT 拦截规则时建立的会话继续使用原读取路径，新增拦截规则后需新建会话。普通报文的出站保持建连时选择，不重新选路。绑定 BT 拦截时，三层 UDP 预匹配转入常规连接路径，避免直接流转发绕过后续检查。UDP 连接封装保留前后协议头预留、MTU、读取开销、缓冲区位置和原关闭接口；检查路径不提供可绕过检查的读取解包接口。

优化后的首包 DHT 若实际命中 BT 拦截，先为普通 UDP 选择出站，再由同一检查器丢弃该首包；同会话后续普通 UDP 保持可用，更早的放行规则仍优先。uTP 新会话识别收紧为完整 SYN，校验扩展长度和报文末尾；单独的 DATA/STATE 不再作为识别依据，避免把 WireGuard 握手误判为 BT。

规则加载和热更新时预计算 BT 候选及说明文本，纯 TCP 条件不启用 UDP 检查器。纯报文解析在路由锁外完成，不为每包重复生成选路调试文本。单包读取、批量读取、同目标批量读取及嗅探缓存都经过过滤，写入继续使用底层快速路径。VLESS 服务端响应头状态使用原子读写，修复并发关闭子会话与计算预留空间的竞态。域名条件仍使用原 DNS 缓存、有效期和禁缓存配置，不增加探测报文。测量见 [DHT 性能记录](../../YZ-Agent/docs/dht-performance.md)。

可选日志上下文接口在准入后捕获当前会话的用户身份；身份撤销后旧会话仍按原用户记录，不修改路由条件使用的上下文。

可选 `adapter.DHTBlockTracker` 只通知已确认的后续报文拦截，不重复登记用户、连接或流量。同一逻辑会话、目标和规则只通知一次，最多保留 256 个通知键；达到上限仍继续拦截。Agent 负责日志队列、用户归属与面板上报，面板不读取报文内容。上游支持等价的严格识别、逐报文匹配和观察接口，且 Agent 双核心联调通过后，可移除本补丁。

验证命令和结果见 [Agent DHT 验证记录](../../YZ-Agent/docs/dht-validation.md)。协议依据：[BEP 5](https://www.bittorrent.org/beps/bep_0005.html)、[BEP 32](https://www.bittorrent.org/beps/bep_0032.html)、[BEP 33](https://www.bittorrent.org/beps/bep_0033.html)、[BEP 43](https://www.bittorrent.org/beps/bep_0043.html)、[BEP 44](https://www.bittorrent.org/beps/bep_0044.html)、[BEP 51](https://www.bittorrent.org/beps/bep_0051.html)。

## 固定基线

- 上游：`https://github.com/SagerNet/sing-box`。
- 官方版本：`v1.14.0`，完整提交 `0b8995879f29a9b98ee027bc17b75e101445b238`。
- 兼容产品版本：`v1.16.0`。产品版本独立于官方基线，不能作为未经修改的官方二进制发布；历史 `v1.14.0-yz.2` 保持原名。
- 本次 Node 适配版本：YZboard-Node `v2.24.2`，正式依赖固定到 `v1.15.1` 或其完整提交。
- 兼容接口和 Mieru 的来源：`cedar2025/sing-box` 提交 `3ac22acd89fcfa3beea866c95a21bdcbdd9032d8`、`2e665cb7e295949ba7c5536f9b7754f94ab78cee`，原基线为 `v1.14.0-alpha.2`。

## 补丁范围与原因

| 范围 | Node 需求与行为 | 后续删除条件 |
| --- | --- | --- |
| `adapter.UpdatableInbound`、各协议用户接口 | 面板用户增删无需重启监听器；保留既有连接 | 官方提供同等用户更新接口，Node 完成迁移 |
| `adapter.RuleUpdater`、`route.Router.UpdateRules` | 使用独立可选接口更新规则和规则集；先准备后替换，失败不破坏旧规则；相同配置不重复重建 | 官方提供具备相同失败语义的路由更新接口 |
| 路由读写锁和规则集快照 | 配置替换、关闭与正在执行的路由匹配相互同步 | 官方原生支持并发热更新 |
| Hysteria2/TUIC 用户身份 | 已认证 QUIC 会话保存身份值，避免用户列表缩短后新流读取旧下标 | 官方用户服务提供可直接使用的稳定身份 |
| 用户表同步与身份快照 | VMess、VLESS、Trojan、Shadowsocks、QUIC 和 Mieru 更新时不与认证或收包竞争；SOCKS/HTTP/Naive 删除全部用户后继续拒绝认证 | 上游提供可并发使用的用户更新接口 |
| 初始网络通知等待 | 启动完成前等待初始化网卡通知，避免刚建立的 QUIC 会话被误当作网络切换关闭 | 上游修复同一初始化时序问题 |
| gRPC 延迟初始化与关闭 | 读取器初始化完成后再发布给读取协程；提前关闭会唤醒等待者并释放迟到的响应，重复关闭只执行一次 | 上游提供相同初始化和关闭同步语义 |
| SS2022 多用户服务关闭 | 只关闭底层连接一次，不读取正在被首次应答初始化的写入器 | 上游修复关闭逻辑并保留现有用户热更新接口 |
| VLESS Vision TLS 缓冲区指针 | 保留 TLS/uTLS 对象指针并使用 `unsafe.Add`，避免地址转整数后还原时触发指针校验；用户认证和已有连接语义保持 | 固定上游及服务端兼容副本提供等价修复，Node 同一组竞态和实际收发检查通过 |
| Mieru 入站、出站及配置注册 | 保留 Node 已支持的 Mieru；使用 `enfein/mieru/v3 v3.29.0` 的 Mux 更新用户并尊重监听地址；停止返回前释放监听端口，监听失败返回错误 | 上游正式提供 Mieru，或 Node 明确停止支持该协议 |
| 通用 TCP 监听的 PROXY v1/v2 接收 | Node 需要在 SS/SS2022 认证和 TLS 握手前还原可信转发机提供的来源；名单按实例隔离，可热更新，空名单和名单外连接不解析；头部检测不阻塞接入循环 | 上游恢复具备可信来源限制的等效接收接口，Node 完成迁移 |

路由热更新没有向官方 `adapter.Router` 主接口增加方法。已经建立的连接继续使用原出站；切换规则只影响后续路由选择。路由读锁在连接转发前释放，空闲 DNS 连接和入站转交不会阻塞热更新。规则集多标签遵守官方格式：本地路径或远程 URL 使用 `{tag}` 占位符，内联规则集仍只有一个标签。

Node 用户流量统计、限速、设备数限制、面板请求和配置生成均留在 YZboard-Node。本仓库不加入面板业务逻辑。

底层依赖的必要服务端兼容副本、固定版本、原始文件校验值与删除条件见 [third_party/README.zh.md](../third_party/README.zh.md)。VMess、VLESS、Trojan 和 Shadowsocks 的认证结果也携带身份值，多路复用流或 UDP 会话不会在用户列表缩短后重新读取旧下标。

Node 为 SOCKS/HTTP 设置 `require_auth: true`，为 Shadowsocks 设置 `multi_user: true`。配置解析会忽略空数组，因此必须用这些显式标记保留认证模式：清空用户后重启仍拒绝所有新连接，随后可以热更新恢复用户。`require_auth` 同样适用于共享 HTTP 配置类型的 mixed 入站。未设置这些标记时保留官方行为，不根据空数组推断认证模式。

## 验证约定

相关核心测试使用：

```bash
go test -tags 'with_quic with_utls with_wireguard with_acme with_clash_api' ./route ./route/rule
```

Node 的运行测试覆盖 VMess、VLESS、Trojan、普通 Shadowsocks、SS2022 AES-128/AES-256、SOCKS、HTTP、AnyTLS、Mieru、Hysteria2 和 TUIC。测试凭据和证书仅在运行时生成，使用回环监听；具体执行结果、目标架构和产物校验值由 Node 兼容矩阵及安装验收记录保存。

2026-09-05 使用 Go 1.26.4 完成上述普通测试；在独立的 Linux amd64 网络空间运行 Node 全部 17 个测试包及本仓库的路由测试包，均通过 race 检查，没有跳过测试或数据竞争报告。路由验证包含规则集事务、并发匹配、初始网络通知、空闲 DNS 连接和 TCP/UDP 入站转交。

正式依赖必须固定到可获取的兼容 Tag 或完整提交；本地路径替换只用于开发验证，不得作为正式发布依赖。

## `v1.15.0` TCP PROXY protocol 兼容补丁

基线为 `09615a105e219076330d9d2a25ea1e2e733d5427`。新增 `common/listener.ProxyProtocolTrust` 和上下文注入接口，嵌入方把单个实例的可信地址交给监听层。未注入策略时保持原监听行为；注入空策略时不信任任何来源，旧的已移除配置开关不会绕过可信名单。

仅在通用 TCP 监听内使用既有 `go-proxyproto v0.8.1` 依赖解析 v1/v2，没有新增依赖版本。接入循环先派发连接协程，再读取来源信息，避免单个慢前置阻塞后续连接。检测超时为 5 秒；可信连接没有协议头时仍可直连，解析错误由连接读取返回。

Node 负责名单来源、面板通信、设备限制和流量统计，本仓库只返回连接来源及剥离协议头后的字节流。原生 UDP、QUIC 和独立组件监听未增加该能力。正式 Node 构建使用已发布的固定版本，不使用本地开发替换。

本地 Go `1.27.0` / Windows、LLVM MinGW Clang `23.1.2` 下，`go test -mod=readonly -race -count=1 -timeout=2m ./common/listener` 通过，覆盖 v1/v2、IPv4/IPv6 来源及 IPv6 前置、普通直连、名单替换和撤销、并发更新、实例隔离、慢连接不阻塞接入及畸形头恢复。Node 使用独立开发模块文件做 SS/SS2022、VLESS、Trojan 实连、流量及来源统计、停止恢复与重载验证，结果见 Node 兼容矩阵。未操作生产服务器。

`YZ compatibility release` 工作流只接受三段语义版本 Tag，固定来源提交，执行监听及既有兼容包的 Linux 竞态测试后构建 amd64、arm64 程序，核对 VCS 与目标架构并生成 Release 校验和。继承的官方镜像和发行版打包工作流仅在官方仓库运行，不向官方命名空间发布 YZ 产物。

## `yz.2` 并发修复

本次基于 `f47d4d565a4371cf46b6c462612fc085f634a6af`，修复 Node 混合内核中转检测发现的 gRPC 读取器发布竞争，以及现有 SS2022 多用户兼容副本中的同类关闭竞争。协议格式、认证身份、用户热更新及路由行为不变。

gRPC 不再通过未经同步的 `reader == nil` 判断是否等待初始化，而是先等待构造时建立的完成信号。初始化和关闭共享状态锁；关闭不等待网络响应，提前关闭时读取立即返回错误，后续到达的响应会被关闭。

SS2022 的读取器和写入器没有独立关闭资源，原 `common.Close` 会沿 `Upstream()` 重复关闭同一连接。现在直接关闭底层连接，不再读取正在发布的写入器。Node 中来自原模块的 SS2022 单用户服务另由 Node 的固定兼容模块使用同一修复；本仓库补丁覆盖既有多用户服务副本。

新增回归测试覆盖并发初始化与读取、初始化前关闭、并发及重复关闭、阻塞读取解除，以及 SS2022 首个普通或批量应答与关闭同时发生的情形。2026-09-08 使用 Go 1.26.4 在 Linux/amd64 验证，旧实现均复现数据竞争；修复后两个包各连续 10 轮 `-race` 通过，共 90 项测试及子测试执行，无数据竞争。最终远程固定依赖的完整 Node 并发验收与构建结果由 Node 兼容矩阵记录。

## `v1.15.1` Vision TLS 指针修复（2026-10-22）

[Release v1.15.1](https://github.com/P0me1oo/YZ-sing-box/releases/tag/v1.15.1) 已发布，来源为 `560816cb3343a9081eff758d988c7fbef767f92b`。[兼容发布流程](https://github.com/P0me1oo/YZ-sing-box/actions/runs/39676739285) 的 Linux 竞态检查、amd64 和 arm64 构建均通过；两个程序的版本、架构、完整来源与 `vcs.modified=false` 已核对。5 份 Release 附件已匿名下载，4 项 SHA256SUMS 全部匹配，amd64 在本机 WSL 实际输出 `sing-box version 1.15.1`。Node 固定引用本版本后构建，服务器无需单独更新该核心。

程序 SHA-256：amd64 为 `3c5452f1b8b7ac4032744a8fe3717f4ab46edd7589ed7f32993591eb83f8c195`，arm64 为 `c6a35807f3878ce95ef8d281daecb1219694fca769df46d4f136816339360cf9`。核验记录位于 `D:/codex-tmp/yz-release-panel-node-20261022-c05033a48076487a857e91002c76ec7b/core/verification.json`；发布后文档提交不移动正式标签。

同步前来源为 `9dbebd69f3a7634b97d44a0948d86b3965cd6cb6`，官方基线仍为 `v1.14.0` / `0b8995879f29a9b98ee027bc17b75e101445b238`。Node `v2.24.1` 完整竞态回归发现，核心内置的 `third_party/sing-vmess/vless/vision.go:87` 同样将 TLS 对象指针经过 `uintptr` 转换，触发指针校验崩溃。Node 的外部模块替换无法改变核心内部副本，因此经用户明确授权修复核心。

仅修改 `vision.go` 与 `vision_utls.go` 的指针传递和缓冲区定位，保留既有用户快照及热更新补丁；新增标准 TLS、uTLS Conn/UConn、垃圾回收回归，并纳入原兼容发布工作流。影响范围是核心内置 VLESS Vision 服务端，Node 外部客户端副本另在 Node 固定；协议、认证、路由、计费和 Node 业务不变。上游及兼容副本提供等价修复并通过现有竞态与真实收发检查后，可以移除本补丁。
