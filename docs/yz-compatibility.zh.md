# YZboard-Node 兼容补丁

## 固定基线

- 上游：`https://github.com/SagerNet/sing-box`。
- 官方版本：`v1.14.0`，完整提交 `0b8995879f29a9b98ee027bc17b75e101445b238`。
- 兼容版本目标：`v1.14.0-yz.1`。该标识包含下列补丁，不能作为未经修改的官方二进制发布。
- Node 使用方：YZboard-Node `v1.13-yz.17`。
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
| Mieru 入站、出站及配置注册 | 保留 Node 已支持的 Mieru；使用 `enfein/mieru/v3 v3.29.0` 的 Mux 更新用户并尊重监听地址；停止返回前释放监听端口，监听失败返回错误 | 上游正式提供 Mieru，或 Node 明确停止支持该协议 |

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
