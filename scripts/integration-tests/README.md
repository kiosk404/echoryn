# Integration Tests Framework

此目录包含 Echoryn 集成测试的支持文件和脚本。

## 目录结构

```
scripts/integration-tests/
├── README.md                        # 本文件
├── fixtures/                        # 测试数据和配置
│   ├── toolset.json                 # 场景 1：工具循环检测用的模拟工具集
│   ├── fault-tools.json             # 场景 2：SubAgent 故障注入工具
│   └── long-conversation.jsonl      # 场景 3：压缩精度测试数据集
├── test-compression.py              # 场景 3：运行长对话压缩测试
└── analyze-compression.py           # 场景 3：分析压缩结果
```

## 快速开始

### 运行全部集成测试
```bash
make test.integration
```

### 运行单个场景
```bash
make test.integration.toolloop       # 场景 1：工具循环检测
make test.integration.subagent       # 场景 2：SubAgent 异常检测
make test.integration.compression    # 场景 3：上下文压缩精度
```

### 生成测试报告
```bash
make test.integration.report
```

输出：`.test-data/integration-test-report.md`

## 详细文档

完整的集成测试规范、验证指标和故障排查指南，请参阅：
**[docs/INTEGRATION_TESTS.md](../../docs/INTEGRATION_TESTS.md)**

## 测试输出

测试运行结果会保存到 `.test-data/` 目录：
- `.test-data/toolloop-result.json` - 场景 1 结果
- `.test-data/subagent-result.json` - 场景 2 结果
- `.test-data/compression-result.json` - 场景 3 结果
- `.test-data/integration-test-report.md` - 最终报告
- `.test-data/baseline/` - 基线数据（用于对比）

## 贡献指南

当实现新的 Harness Engineering 机制时，应在 `docs/INTEGRATION_TESTS.md` 中新增测试场景。

新场景模板：
1. 📌 测试目的
2. 🔧 前置条件
3. 📋 测试步骤（具体命令）
4. 📊 验证指标（表格）
5. ✅ 通过条件
6. ❌ 失败排查路径
7. 🔗 相关代码位置
