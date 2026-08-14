# 贡献指南

[English](CONTRIBUTING.md) | **简体中文**

感谢你愿意改进 Bursar。提交代码前请先阅读[行为准则](CODE_OF_CONDUCT.zh-CN.md)。安全漏洞不要提交公开 Issue，请按[安全策略](SECURITY.zh-CN.md)私下报告。

## 开始之前

1. 搜索现有 Issue 和 Pull Request，确认问题尚未被处理。
2. 较大的功能、数据模型变更或不兼容改动，先创建 Issue 说明动机、范围和迁移方案。
3. Fork 仓库并从最新 `main` 创建短生命周期分支。

## 本地开发

所需工具和启动方式见[开发指南](docs/zh-CN/development.md)。常用验证命令：

```bash
go test ./controller/... ./node-agent/...
go vet ./controller/... ./node-agent/...
pnpm --dir web install --frozen-lockfile
pnpm --dir web build
bash -n scripts/*.sh
```

只修改文档时，至少运行：

```bash
bash scripts/check_docs.sh
```

## 变更要求

- 保持实现简单、明确，并遵循现有目录和命名约定。
- 行为变更应补充或更新测试；无法自动测试时，在 PR 中写清人工验证方法。
- 配置、API、管理员页面或运维流程变化时，同步更新英文主文档和中文翻译。
- 不要提交真实 token、私钥、数据库 DSN、主机清单、用户数据、备份或构建产物。
- 新依赖必须说明用途，并确认许可证与 Apache-2.0 分发兼容。
- 数据库变更通过新的顺序迁移文件实现；不要改写已经发布的迁移。

## 提交与 Pull Request

推荐使用简洁的 Conventional Commits 风格：

```text
feat: add node maintenance mode
fix: prevent duplicate usage billing
docs: document first-run setup
```

Pull Request 应包含问题背景、解决方案、用户可见变化、兼容性影响、测试结果和迁移说明。涉及界面时可附截图，但不得包含真实账号、主机或业务数据。

提交贡献即表示你同意按仓库的 [Apache License 2.0](LICENSE) 提供该贡献。
