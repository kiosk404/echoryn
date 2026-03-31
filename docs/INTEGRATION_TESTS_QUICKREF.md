# 集成测试快速参考

## 📌 核心理念

Echoryn 的集成测试验证 **6 个 Harness Engineering 核心机制** 在真实工作流中是否可靠：

- ✅ **工具循环检测** - 防止 Agent 死循环
- ✅ **SubAgent 观察者** - 监控子 Agent 异常
- ✅ **上下文压缩** - Token 优化
- ⏳ **多渠道路由** - IM 网关故障转移（待实现）
- ⏳ **插件槽位** - 插件冲突解决（待实现）
- ⏳ **记忆系统** - FTS5 + 向量搜索（待实现）

## 🚀 一键运行

```bash
# 运行全部 3 个场景（约 25 分钟）
make test.integration

# 生成报告
make test.integration.report

# 生成的报告位置
cat .test-data/integration-test-report.md
```

## 🎯 场景快速测试

```bash
# 场景 1：工具循环检测（5 分钟）
make test.integration.toolloop

# 场景 2：SubAgent 异常检测（8 分钟）
make test.integration.subagent

# 场景 3：上下文压缩精度（10 分钟）
make test.integration.compression
```

## 📊 验证指标速查

| 场景 | 关键指标 | 预期值 |
|------|---------|-------|
| 工具循环检测 | 误报率 | < 2% |
| | 漏报率 | < 1% |
| | 检测延迟 | < 500ms |
| SubAgent 异常 | 检测时间 | < 20s |
| | 误报率 | < 2% |
| | 恢复成功率 | > 95% |
| 上下文压缩 | 压缩率 | 0.55~0.65 |
| | 精度保留 | > 90% |
| | 任务完成率 | > 95% |

## 📖 详细文档

完整的测试规范、故障排查、CI 集成指南：
👉 **[docs/INTEGRATION_TESTS.md](docs/INTEGRATION_TESTS.md)**

## 💡 何时运行

- ✅ **开发完新功能后** - 验证设计是否有效
- ✅ **提交 PR 前** - 确保无回归
- ✅ **发版前** - 全量验证
- ✅ **用户报告问题** - 复现 + 修复验证

## 🔍 故障排查

测试失败？查看对应场景的"失败排查路径"：

```bash
# 打开对应场景的文档
cat docs/INTEGRATION_TESTS.md | grep -A 20 "失败排查路径"
```

## 📝 扩展新场景

新增机制后，在 `docs/INTEGRATION_TESTS.md` 添加新场景：

1. 📌 测试目的
2. 🔧 前置条件
3. 📋 测试步骤
4. 📊 验证指标
5. ✅ 通过条件
6. ❌ 失败排查路径

然后在 `Makefile` 添加新目标：
```makefile
.PHONY: test.integration.newscene
test.integration.newscene:
	@echo "Running new scene..."
```

更新 `test.integration` 目标加入新场景。

---

**最后更新**：2025-02-01 | [完整集成测试文档](docs/INTEGRATION_TESTS.md)
