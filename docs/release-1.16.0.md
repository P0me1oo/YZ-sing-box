# YZ-sing-box v1.16.0 发布

维护者：P0me1oo。日期：2026-11-16。

[v1.16.0](https://github.com/P0me1oo/YZ-sing-box/releases/tag/v1.16.0) 已发布并设为 Latest，固定源码 `ad0c0cfbc7c3827f3058aa666d9f1a3b0e0fd085`。包含全部本地 DHT 识别、逐报文路由、缓冲生命周期及相关兼容修复；上游基线和原有补丁保留。

DHT、嗅探、路由、规则、VLESS 五包竞态检查通过；修复日志上下文接口后整组补验通过。

Linux amd64/arm64 两份程序及构建元数据从固定提交生成，已核对架构、CGO=0、VCS revision 和 vcs.modified=false。五份附件全部匿名下载，与本地产物及 GitHub 摘要一致。Agent v2.38.0 已锁定该核心；核心不单独替换运行服务器程序。

GitHub 自动工作流因账号账单锁定未执行，未将自动检查计为通过，使用本机验证和固定构建。不发布 Docker 镜像；发布标签及附件不因后补记录改变。详见[核验数据](release-1.16.0-verification.json)。
