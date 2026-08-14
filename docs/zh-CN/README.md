# Bursar 文档

[English](../README.md) | **简体中文**

本目录是 Bursar 的完整文档集。英文文档是源语言，`docs/zh-CN/` 下是对应的简体中文翻译。

## 选择阅读路径

### 评估项目

1. [项目 README](../../README.zh-CN.md)
2. [架构](architecture.md)
3. [安全模型](security.md)
4. [快速入门](getting-started.md)

### 部署与运维

1. [安装指南](installation.md)
2. [配置参考](configuration.md)
3. [首次 Setup](first-run-setup.md)
4. [Node Agent](node-agent.md)
5. [上线检查清单](go-live-checklist.md)
6. [运维指南](operations.md)
7. [备份与 HA](backup-and-ha.md)
8. [故障排查](troubleshooting.md)

### 使用与管理平台

- [管理员指南](admin-guide.md)
- [用户指南](user-guide.md)
- [API 参考](api-reference.md)

### 参与贡献

- [开发指南](development.md)
- [容器与源码部署](installation.md)
- [贡献指南](../../CONTRIBUTING.zh-CN.md)
- [安全策略](../../SECURITY.zh-CN.md)
- [行为准则](../../CODE_OF_CONDUCT.zh-CN.md)
- [变更日志](../../CHANGELOG.zh-CN.md)

## 文档归属

| 主题 | 权威文件 |
| --- | --- |
| 产品概览与五分钟上手 | `README.md` |
| 组件、数据流、信任边界 | `architecture.md` |
| 安装与生命周期 | `installation.md`、`operations.md` |
| 启动 YAML 与运行时设置 | `configuration.md`、`first-run-setup.md` |
| 节点行为 | `node-agent.md` |
| Web 工作流 | `admin-guide.md`、`user-guide.md` |
| HTTP 契约 | `api-reference.md` |
| 安全与漏洞披露 | `security.md`、根目录 `SECURITY.md` |

行为发生变化时，请在同一个 Pull Request 中同时更新英文源文档和对应的中文文档。提交前请运行 `bash scripts/check_docs.sh`。
