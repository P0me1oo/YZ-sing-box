# 用户热更新所需的协议兼容代码

这些目录保存固定依赖版本中的协议实现，供本仓库的服务端使用。原始模块的用户更新入口没有完整的并发同步；仅在 Node 的更新调用外加锁，无法保护协议库内部的认证协程和 UDP 接收协程。兼容代码让用户表以完整快照发布，已经建立的连接继续保留认证时的身份和密钥。

| 目录 | 原始模块与版本 | 保留范围 | 本地修改 |
| --- | --- | --- | --- |
| `sing-vmess` | `github.com/sagernet/sing-vmess v0.2.8-0.20250909125414-3aed155119a1` | VMess、VLESS 及编译所需的同包辅助代码 | 同步用户表发布、VMess 旧版密钥刷新和关闭；握手读取不可变快照 |
| `sing-shadowsocks` | `github.com/sagernet/sing-shadowsocks v0.2.8` | `shadowaead`、`shadowaead_2022` | 先构造完整用户表再发布；TCP、UDP 和 UDP 会话初始化使用同一快照 |
| `sing-quic` | `github.com/sagernet/sing-quic v0.7.0-beta.4` | TUIC、Hysteria2 服务端及同包辅助代码 | 用户与密码在同一把锁下发布和读取，保留 QUIC 监听器与已有会话 |
| `mieru` | `github.com/enfein/mieru/v3 v3.29.0` | `pkg/protocol` | 同步 Mux 与 UDP Underlay 的用户快照，TCP 已有连接保留原用户表；启动时向调用方返回监听失败 |

各目录保留原项目许可证和版权声明。`sources.json` 记录每份原始文件的模块版本、相对路径和 SHA-256；其中校验值用于核对修改前的来源，不能当成本地修改后文件的校验值。导入路径调整只用于引用本目录内需要一同编译的辅助代码。

Node 的测试客户端继续使用官方协议客户端，通过真实 TCP、UDP 收发验证互通；TUIC、Hysteria2 客户端和 Mieru API 的其余实现仍来自固定的原模块。这里不加入面板、流量扣费、安装或节点管理逻辑。

升级这些依赖时，应先按 `sources.json` 比较原始版本与目标固定版本，再迁移仍然需要的同步修改，并运行用户增删、空用户列表、已有连接、新流、UDP、重载和 race 检查。上游提供具备同等并发语义的更新入口后，应恢复直接导入上游包并移除对应副本。
